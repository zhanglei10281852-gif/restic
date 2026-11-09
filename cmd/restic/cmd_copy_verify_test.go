package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func testRunCopyVerify(t testing.TB, srcGopts global.Options, dstGopts global.Options) (string, string, error) {
	gopts := srcGopts
	gopts.Repo = dstGopts.Repo
	gopts.Password = dstGopts.Password
	gopts.InsecureNoPassword = dstGopts.InsecureNoPassword
	// make sure the verification progress messages are part of the captured output
	gopts.Quiet = false
	gopts.Verbosity = 1
	copyOpts := CopyOptions{
		SecondaryRepoOptions: global.SecondaryRepoOptions{
			Repo:               srcGopts.Repo,
			Password:           srcGopts.Password,
			InsecureNoPassword: srcGopts.InsecureNoPassword,
		},
		Verify: true,
	}

	stdout, stderr, err := withCaptureStdoutStderr(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runCopy(ctx, copyOpts, gopts, nil, gopts.Term)
	})
	return stdout.String(), stderr.String(), err
}

// setupCopyVerifyRepo initializes the source repository and creates a directory
// of plain test files. It avoids the tar-based test fixture so that the test
// also works on platforms that cannot create the symlinks contained in it.
func setupCopyVerifyRepo(t testing.TB, env *testEnvironment) string {
	t.Helper()
	testRunInit(t, env.gopts)

	base := filepath.Join(env.testdata, "data")
	rtest.OK(t, os.MkdirAll(filepath.Join(base, "sub"), 0o700))
	rtest.OK(t, os.WriteFile(filepath.Join(base, "file1"), []byte("hello world\n"), 0o600))
	rtest.OK(t, os.WriteFile(filepath.Join(base, "sub", "file2"), bytes.Repeat([]byte("restic "), 1000), 0o600))
	return base
}

func TestCopyVerify(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	base := setupCopyVerifyRepo(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{base}, opts, env.gopts)
	rtest.OK(t, os.WriteFile(filepath.Join(base, "file3"), []byte("second snapshot\n"), 0o600))
	testRunBackup(t, "", []string{base}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	testRunInit(t, env2.gopts)
	stdout, stderr, err := testRunCopyVerify(t, env.gopts, env2.gopts)
	rtest.Assert(t, err == nil, "copy --verify failed: %v\n%v\n%v", err, stdout, stderr)
	rtest.Assert(t, strings.Contains(stdout, "snapshot"), "missing per-snapshot output, got %q", stdout)
	rtest.Assert(t, strings.Contains(stdout, "verified"), "missing verified report, got %q", stdout)
	rtest.Assert(t, strings.Contains(stdout, "verified 2 snapshots"), "missing verification summary, got %q", stdout)

	// run again: all snapshots are skipped because they were already copied,
	// verification must still check every single snapshot
	stdout, stderr, err = testRunCopyVerify(t, env.gopts, env2.gopts)
	rtest.Assert(t, err == nil, "repeated copy --verify failed: %v\n%v\n%v", err, stdout, stderr)
	rtest.Assert(t, strings.Contains(stdout, "verified 2 snapshots"), "missing verification summary, got %q", stdout)

	testRunCheck(t, env2.gopts)
}

func TestCopyVerifyMissingDataBlob(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	base := setupCopyVerifyRepo(t, env)
	testRunBackup(t, "", []string{base}, BackupOptions{}, env.gopts)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	// remove the data packfiles while the index still references the blobs
	removePacksExcept(env2.gopts, t, restic.NewIDSet(), false)

	_, _, err := testRunCopyVerify(t, env.gopts, env2.gopts)
	rtest.Assert(t, err != nil, "expected verification to fail for missing data blob")
	rtest.Assert(t, strings.Contains(err.Error(), "snapshot"), "error does not mention the snapshot: %v", err)
	rtest.Assert(t, strings.Contains(err.Error(), "path"), "error does not mention the path: %v", err)
}

// tamperingLoadRepo modifies the plaintext of a single blob when it is loaded,
// simulating bitrot that is not caught by the repository-internal checks.
type tamperingLoadRepo struct {
	*repository.Repository
	tamper restic.BlobHandle
}

func (r *tamperingLoadRepo) LoadBlob(ctx context.Context, bh restic.BlobHandle, buf []byte) ([]byte, error) {
	plaintext, err := r.Repository.LoadBlob(ctx, bh, buf)
	if err != nil {
		return nil, err
	}
	if bh == r.tamper {
		plaintext = append(append([]byte{}, plaintext...), 0)
	}
	return plaintext, nil
}

func TestVerifyTreeDetectsTamperedBlobs(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	base := setupCopyVerifyRepo(t, env)
	testRunBackup(t, "", []string{base}, BackupOptions{}, env.gopts)
	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, srcRepo, unlockSrc, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlockSrc()

		dstGopts := env2.gopts
		dstGopts.Term = gopts.Term
		_, dstRepo, unlockDst, err := openWithReadLock(ctx, dstGopts, false, printer)
		rtest.OK(t, err)
		defer unlockDst()

		// list snapshots before loading the index to satisfy the ordered test backend
		srcSn := firstTestSnapshot(ctx, t, srcRepo)
		dstSn := firstTestSnapshot(ctx, t, dstRepo)

		rtest.OK(t, srcRepo.LoadIndex(ctx, restic.NoopTerminalCounterFactory))
		rtest.OK(t, dstRepo.LoadIndex(ctx, restic.NoopTerminalCounterFactory))

		// collect one data blob and the root tree blob of the destination repo
		var dataBlob restic.BlobHandle
		rtest.OK(t, dstRepo.ListBlobs(ctx, func(pb restic.PackBlob) {
			if pb.Handle().Type == restic.DataBlob && dataBlob.ID.IsNull() {
				dataBlob = pb.Handle()
			}
		}))
		rtest.Assert(t, !dataBlob.ID.IsNull(), "expected to find a data blob in the destination repository")
		rootTree := restic.BlobHandle{Type: restic.TreeBlob, ID: *dstSn.Tree}

		tamperedData := &tamperingLoadRepo{Repository: dstRepo, tamper: dataBlob}
		err = verifyTree(ctx, srcRepo, tamperedData, *srcSn.Tree, *dstSn.Tree, "/", dstRepo.NewAssociatedBlobSet())
		rtest.Assert(t, err != nil, "expected data blob hash mismatch to be detected")
		rtest.Assert(t, strings.Contains(err.Error(), "content hash does not match"), "unexpected error: %v", err)
		rtest.Assert(t, strings.Contains(err.Error(), "path"), "error does not mention the path: %v", err)

		tamperedTree := &tamperingLoadRepo{Repository: dstRepo, tamper: rootTree}
		err = verifyTree(ctx, srcRepo, tamperedTree, *srcSn.Tree, *dstSn.Tree, "/", dstRepo.NewAssociatedBlobSet())
		rtest.Assert(t, err != nil, "expected tree blob hash mismatch to be detected")
		rtest.Assert(t, strings.Contains(err.Error(), "tree blob"), "unexpected error: %v", err)
		rtest.Assert(t, strings.Contains(err.Error(), "content hash does not match"), "unexpected error: %v", err)
		return nil
	}))
}

func firstTestSnapshot(ctx context.Context, t testing.TB, repo restic.Repository) *data.Snapshot {
	t.Helper()
	lister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	rtest.OK(t, err)

	var sn *data.Snapshot
	rtest.OK(t, data.ForAllSnapshots(ctx, lister, repo, nil, func(_ restic.ID, s *data.Snapshot, err error) error {
		if err != nil {
			return err
		}
		if sn == nil {
			sn = s
		}
		return nil
	}))
	rtest.Assert(t, sn != nil, "repository contains no snapshots")
	return sn
}

func TestFindDestinationSnapshot(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	base := setupCopyVerifyRepo(t, env)
	testRunBackup(t, "", []string{base}, BackupOptions{}, env.gopts)
	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	var srcSn, dstSn *data.Snapshot
	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, srcRepo, unlockSrc, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlockSrc()

		dstGopts := env2.gopts
		dstGopts.Term = gopts.Term
		_, dstRepo, unlockDst, err := openWithReadLock(ctx, dstGopts, false, printer)
		rtest.OK(t, err)
		defer unlockDst()

		srcSn = firstTestSnapshot(ctx, t, srcRepo)
		dstSn = firstTestSnapshot(ctx, t, dstRepo)
		return nil
	}))

	// no destination snapshot at all
	_, err := findDestinationSnapshot(srcSn, map[restic.ID][]*data.Snapshot{})
	rtest.Assert(t, err != nil, "expected an error for a missing destination snapshot")
	rtest.Assert(t, strings.Contains(err.Error(), "no corresponding snapshot"), "unexpected error: %v", err)

	// exactly one matching destination snapshot
	dstSnapshotByOriginal := make(map[restic.ID][]*data.Snapshot)
	addDestinationSnapshot(dstSnapshotByOriginal, dstSn)
	found, err := findDestinationSnapshot(srcSn, dstSnapshotByOriginal)
	rtest.OK(t, err)
	rtest.Equals(t, dstSn.ID(), found.ID())

	// the destination snapshot is present twice with identical metadata
	srcOriginal := *srcSn.ID()
	if srcSn.Original != nil {
		srcOriginal = *srcSn.Original
	}
	duplicates := map[restic.ID][]*data.Snapshot{
		srcOriginal: {dstSn, dstSn},
	}
	_, err = findDestinationSnapshot(srcSn, duplicates)
	rtest.Assert(t, err != nil, "expected an error for duplicate destination snapshots")
	rtest.Assert(t, strings.Contains(err.Error(), "multiple corresponding snapshots"), "unexpected error: %v", err)
}

func TestCompareNodes(t *testing.T) {
	base := testVerifyNode()

	// identical nodes compare without error
	identical := testVerifyNode()
	rtest.OK(t, compareNodes(&base, &identical, "/file"))

	modTime := time.Date(2025, 6, 7, 8, 9, 10, 0, time.UTC)
	otherContent := restic.TestParseID("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	otherSubtree := restic.TestParseID("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")

	testCases := []struct {
		name        string
		modify      func(node *data.Node)
		errContains string
	}{
		{"name", func(n *data.Node) { n.Name = "other" }, "name mismatch"},
		{"node type", func(n *data.Node) { n.Type = data.NodeTypeDir }, "node type mismatch"},
		{"mode", func(n *data.Node) { n.Mode = 0o600 }, "mode mismatch"},
		{"size", func(n *data.Node) { n.Size = 42 }, "size mismatch"},
		{"mtime", func(n *data.Node) { n.ModTime = modTime }, "modification time mismatch"},
		{"atime", func(n *data.Node) { n.AccessTime = modTime }, "access time mismatch"},
		{"ctime", func(n *data.Node) { n.ChangeTime = modTime }, "change time mismatch"},
		{"link target", func(n *data.Node) { n.LinkTarget = "other-target" }, "link target mismatch"},
		{"device", func(n *data.Node) { n.Device = 99 }, "device mismatch"},
		{"device id", func(n *data.Node) { n.DeviceID = 99 }, "device id mismatch"},
		{"content length", func(n *data.Node) { n.Content = restic.IDs{base.Content[0]} }, "content blob sequence length mismatch"},
		{"content id", func(n *data.Node) { n.Content = restic.IDs{base.Content[0], otherContent} }, "content blob at position 1 mismatch"},
		{"extended attributes", func(n *data.Node) {
			n.ExtendedAttributes = []data.ExtendedAttribute{{Name: "x", Value: []byte("other")}}
		}, "extended attributes mismatch"},
		{"generic attributes", func(n *data.Node) {
			n.GenericAttributes = map[data.GenericAttributeType]json.RawMessage{"windows.creation_time": json.RawMessage(`456`)}
		}, "generic attributes mismatch"},
		{"subtree", func(n *data.Node) { n.Subtree = &otherSubtree }, "subtree mismatch"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			other := testVerifyNode()
			tc.modify(&other)
			err := compareNodes(&base, &other, "/file")
			rtest.Assert(t, err != nil, "expected an error for %s mismatch", tc.name)
			rtest.Assert(t, strings.Contains(err.Error(), tc.errContains), "expected %q in error, got %v", tc.errContains, err)
			rtest.Assert(t, strings.Contains(err.Error(), "/file"), "error does not mention the node path: %v", err)
		})
	}
}

func testVerifyNode() data.Node {
	subtree := restic.TestParseID("2222222222222222222222222222222222222222222222222222222222222222")
	return data.Node{
		Name:       "file",
		Type:       data.NodeTypeFile,
		Mode:       0o644,
		ModTime:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		AccessTime: time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC),
		ChangeTime: time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC),
		UID:        1,
		GID:        2,
		User:       "user",
		Group:      "group",
		Inode:      10,
		DeviceID:   20,
		Links:      1,
		Size:       100,
		LinkTarget: "target",
		Device:     5,
		Content: restic.IDs{
			restic.TestParseID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			restic.TestParseID("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		},
		ExtendedAttributes: []data.ExtendedAttribute{{Name: "x", Value: []byte("y")}},
		GenericAttributes: map[data.GenericAttributeType]json.RawMessage{
			"windows.creation_time": json.RawMessage(`123`),
		},
		Subtree: &subtree,
	}
}
