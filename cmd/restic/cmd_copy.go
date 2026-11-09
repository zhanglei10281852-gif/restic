package main

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"path"
	"reflect"
	"sync"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newCopyCommand(globalOptions *global.Options) *cobra.Command {
	var opts CopyOptions
	cmd := &cobra.Command{
		Use:   "copy [flags] [snapshotID ...]",
		Short: "Copy snapshots from one repository to another",
		Long: `
The "copy" command copies one or more snapshots from one repository to another.

NOTE: This process will have to both download (read) and upload (write) the
entire snapshot(s) due to the different encryption keys used in the source and
destination repositories. This /may incur higher bandwidth usage and costs/ than
expected during normal backup runs.

NOTE: The copying process does not re-chunk files, which may break deduplication
between the files copied and files already stored in the destination repository.
This means that copied files, which existed in both the source and destination
repository, /may occupy up to twice their space/ in the destination repository.
This can be mitigated by the "--copy-chunker-params" option when initializing a
new destination repository using the "init" command.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runCopy(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// CopyOptions bundles all options for the copy command.
type CopyOptions struct {
	global.SecondaryRepoOptions
	data.SnapshotFilter
	Verify bool
}

func (opts *CopyOptions) AddFlags(f *pflag.FlagSet) {
	opts.SecondaryRepoOptions.AddFlags(f, "destination", "to copy snapshots from")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)

	f.BoolVar(&opts.Verify, "verify", false, "verify that the snapshots are correctly stored in the destination repository")
}

var errSentinelEndIteration = errors.New("end iteration")

// collectAllSnapshots: select all snapshot trees to be copied
func collectAllSnapshots(ctx context.Context, opts CopyOptions,
	srcSnapshotLister restic.Lister, srcRepo restic.Repository,
	dstSnapshotByOriginal map[restic.ID][]*data.Snapshot, args []string, printer restic.Printer,
) iter.Seq2[*data.Snapshot, error] {
	return func(yield func(*data.Snapshot, error) bool) {
		err := opts.SnapshotFilter.FindAll(ctx, srcSnapshotLister, srcRepo, args, func(_ string, sn *data.Snapshot, err error) error {
			// check whether the destination has a snapshot with the same persistent ID which has similar snapshot fields
			if err != nil {
				if !yield(nil, err) {
					return errSentinelEndIteration
				}
				return nil
			}
			srcOriginal := *sn.ID()
			if sn.Original != nil {
				srcOriginal = *sn.Original
			}
			if originalSns, ok := dstSnapshotByOriginal[srcOriginal]; ok {
				isCopy := false
				for _, originalSn := range originalSns {
					if similarSnapshots(originalSn, sn) {
						printer.V("\n%v", sn)
						printer.V("skipping source snapshot %s, was already copied to snapshot %s", sn.ID().Str(), originalSn.ID().Str())
						isCopy = true
						break
					}
				}
				if isCopy {
					return nil
				}
			}
			if !yield(sn, nil) {
				return errSentinelEndIteration
			}
			return nil
		})
		if err != nil && !errors.Is(err, errSentinelEndIteration) {
			yield(nil, err)
		}
	}
}

func runCopy(ctx context.Context, opts CopyOptions, gopts global.Options, args []string, term ui.Terminal) error {
	printer := progress.NewTerminalPrinter(false, gopts.Verbosity, term)
	secondaryGopts, isFromRepo, err := opts.SecondaryRepoOptions.FillGlobalOpts(ctx, gopts, "destination")
	if err != nil {
		return err
	}
	if isFromRepo {
		// swap global options, if the secondary repo was set via from-repo
		gopts, secondaryGopts = secondaryGopts, gopts
	}

	ctx, srcRepo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	ctx, dstRepo, unlock, err := openWithAppendLock(ctx, secondaryGopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

	srcSnapshotLister, err := restic.MemorizeList(ctx, srcRepo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	dstSnapshotLister, err := restic.MemorizeList(ctx, dstRepo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	debug.Log("Loading source index")
	if err := srcRepo.LoadIndex(ctx, printer); err != nil {
		return err
	}
	debug.Log("Loading destination index")
	if err := dstRepo.LoadIndex(ctx, printer); err != nil {
		return err
	}

	dstSnapshotByOriginal := make(map[restic.ID][]*data.Snapshot)
	err = opts.SnapshotFilter.FindAll(ctx, dstSnapshotLister, dstRepo, nil, func(_ string, sn *data.Snapshot, err error) error {
		if err != nil {
			return err
		}
		addDestinationSnapshot(dstSnapshotByOriginal, sn)
		return nil
	})
	if err != nil {
		return err
	}

	selectedSnapshots := collectAllSnapshots(ctx, opts, srcSnapshotLister, srcRepo, dstSnapshotByOriginal, args, printer)

	savedSnapshotIDs, err := copyTreeBatched(ctx, srcRepo, dstRepo, selectedSnapshots, printer)
	if err != nil {
		return err
	}

	if opts.Verify {
		// the snapshots saved during this run are not part of the destination
		// snapshot list loaded above, reload and add them to the lookup map
		for _, newID := range savedSnapshotIDs {
			dstSn, err := data.LoadSnapshot(ctx, dstRepo, newID)
			if err != nil {
				return err
			}
			addDestinationSnapshot(dstSnapshotByOriginal, dstSn)
		}
		if err := verifySnapshots(ctx, opts, srcSnapshotLister, srcRepo, dstRepo, dstSnapshotByOriginal, args, printer); err != nil {
			return err
		}
	}

	return ctx.Err()
}

func similarSnapshots(sna *data.Snapshot, snb *data.Snapshot) bool {
	// everything except Parent and Original must match
	if !sna.Time.Equal(snb.Time) || !sna.Tree.Equal(*snb.Tree) || sna.Hostname != snb.Hostname ||
		sna.Username != snb.Username || sna.UID != snb.UID || sna.GID != snb.GID ||
		len(sna.Paths) != len(snb.Paths) || len(sna.Excludes) != len(snb.Excludes) ||
		len(sna.Tags) != len(snb.Tags) {
		return false
	}
	if !sna.HasPaths(snb.Paths) || !sna.HasTags(snb.Tags) {
		return false
	}
	for i, a := range sna.Excludes {
		if a != snb.Excludes[i] {
			return false
		}
	}
	return true
}

// copyTreeBatched copies multiple snapshots in one go. Snapshots are written after
// data equivalent to at least 10 packfiles was written. It returns the IDs of the
// snapshots that were saved in the destination repository.
func copyTreeBatched(ctx context.Context, srcRepo *repository.Repository, dstRepo restic.Repository,
	selectedSnapshots iter.Seq2[*data.Snapshot, error], printer restic.Printer) (restic.IDs, error) {

	// remember already processed trees across all snapshots
	visitedTrees := srcRepo.NewAssociatedBlobSet()

	targetSize := uint64(dstRepo.PackSize()) * 100
	minDuration := 1 * time.Minute

	// use pull-based iterator to allow iteration in multiple steps
	next, stop := iter.Pull2(selectedSnapshots)
	defer stop()

	var savedSnapshotIDs restic.IDs

	for {
		var batch []*data.Snapshot
		batchSize := uint64(0)
		startTime := time.Now()

		// call WithBlobUploader() once and then loop over all selectedSnapshots
		err := dstRepo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
			for batchSize < targetSize || time.Since(startTime) < minDuration {
				sn, err, ok := next()
				if err != nil {
					return err
				}
				if !ok {
					break
				}

				batch = append(batch, sn)

				printer.P("\n%v", sn)
				printer.P("  copy started, this may take a while...")
				sizeBlobs, err := copyTree(ctx, srcRepo, dstRepo, visitedTrees, *sn.Tree, printer, uploader)
				if err != nil {
					return err
				}
				debug.Log("tree copied")
				batchSize += sizeBlobs
			}

			return nil
		})
		if err != nil {
			return nil, err
		}

		// if no snapshots were processed in this batch, we're done
		if len(batch) == 0 {
			break
		}

		// add a newline to separate saved snapshot messages from the other messages
		if len(batch) > 1 {
			printer.P("")
		}
		// save all the snapshots
		for _, sn := range batch {
			newID, err := copySaveSnapshot(ctx, sn, dstRepo, printer)
			if err != nil {
				return nil, err
			}
			savedSnapshotIDs = append(savedSnapshotIDs, newID)
		}
	}

	return savedSnapshotIDs, nil
}

func copyTree(ctx context.Context, srcRepo *repository.Repository, dstRepo restic.Repository,
	visitedTrees restic.AssociatedBlobSet, rootTreeID restic.ID, printer restic.Printer, uploader restic.BlobSaverWithAsync) (uint64, error) {

	copyBlobs := srcRepo.NewAssociatedBlobSet()
	packList := restic.NewIDSet()
	var lock sync.Mutex

	enqueue := func(h restic.BlobHandle) {
		lock.Lock()
		defer lock.Unlock()
		if _, ok := dstRepo.LookupBlobSize(h); !ok {
			pb := srcRepo.LookupBlob(h)
			copyBlobs.Insert(h)
			for _, p := range pb {
				packList.Insert(p.PackID())
			}
		}
	}

	err := data.StreamTrees(ctx, srcRepo, restic.IDs{rootTreeID}, restic.NoopCounter, func(treeID restic.ID) bool {
		handle := restic.BlobHandle{ID: treeID, Type: restic.TreeBlob}
		visited := visitedTrees.Has(handle)
		visitedTrees.Insert(handle)
		return visited
	}, func(treeID restic.ID, err error, nodes data.TreeNodeIterator) error {
		if err != nil {
			return fmt.Errorf("LoadTree(%v) returned error %v", treeID.Str(), err)
		}

		// copy raw tree bytes to avoid problems if the serialization changes
		enqueue(restic.BlobHandle{ID: treeID, Type: restic.TreeBlob})

		for item := range nodes {
			if item.Error != nil {
				return item.Error
			}
			// Recursion into directories is handled by StreamTrees
			// Copy the blobs for this file.
			for _, blobID := range item.Node.Content {
				enqueue(restic.BlobHandle{Type: restic.DataBlob, ID: blobID})
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	sizeBlobs := copyStats(srcRepo, copyBlobs, packList, printer)
	bar := printer.NewCounter("packs copied")
	err = repository.CopyBlobs(ctx, srcRepo, dstRepo, uploader, packList, copyBlobs, bar, printer.P)
	if err != nil {
		return 0, errors.Fatalf("%s", err)
	}
	return sizeBlobs, nil
}

// copyStats: print statistics for the blobs to be copied
func copyStats(srcRepo restic.Repository, copyBlobs restic.AssociatedBlobSet, packList restic.IDSet, printer restic.Printer) uint64 {
	// count and size
	countBlobs := 0
	sizeBlobs := uint64(0)
	for blob := range copyBlobs.Keys() {
		for _, pb := range srcRepo.LookupBlob(blob) {
			countBlobs++
			sizeBlobs += uint64(pb.CiphertextLength())
			break
		}
	}

	printer.V("  copy %d blobs with disk size %s in %d packfiles\n",
		countBlobs, ui.FormatBytes(uint64(sizeBlobs)), len(packList))
	return sizeBlobs
}

func copySaveSnapshot(ctx context.Context, sn *data.Snapshot, dstRepo restic.Repository, printer restic.Printer) (restic.ID, error) {
	sn.Parent = nil // Parent does not have relevance in the new repo.
	// Use Original as a persistent snapshot ID
	if sn.Original == nil {
		sn.Original = sn.ID()
	}
	newID, err := data.SaveSnapshot(ctx, dstRepo, sn)
	if err != nil {
		return restic.ID{}, err
	}
	printer.P("snapshot %s saved, copied from source snapshot %s", newID.Str(), sn.ID().Str())
	return newID, nil
}

// verifySnapshots verifies that each snapshot selected by the snapshot filter
// has exactly one corresponding snapshot in the destination repository and
// that the complete content of all snapshots is stored correctly there. This
// also covers snapshots which were skipped because they were already copied.
func verifySnapshots(ctx context.Context, opts CopyOptions,
	srcSnapshotLister restic.Lister, srcRepo restic.Repository, dstRepo restic.Repository,
	dstSnapshotByOriginal map[restic.ID][]*data.Snapshot, args []string, printer restic.Printer) error {

	// remember blobs that were already verified to avoid reading them again
	// for snapshots that share parts of their trees
	verifiedBlobs := dstRepo.NewAssociatedBlobSet()

	verifiedCount := 0
	err := opts.SnapshotFilter.FindAll(ctx, srcSnapshotLister, srcRepo, args, func(_ string, sn *data.Snapshot, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		dstSn, err := findDestinationSnapshot(sn, dstSnapshotByOriginal)
		if err != nil {
			return fmt.Errorf("snapshot %s: %w", sn.ID().Str(), err)
		}

		if err := verifyTree(ctx, srcRepo, dstRepo, *sn.Tree, *dstSn.Tree, "/", verifiedBlobs); err != nil {
			return fmt.Errorf("snapshot %s: %w", sn.ID().Str(), err)
		}
		printer.P("snapshot %s verified", sn.ID().Str())
		verifiedCount++
		return nil
	})
	if err != nil {
		return err
	}

	printer.P("verified %d snapshots", verifiedCount)
	return nil
}

// addDestinationSnapshot indexes a destination snapshot by its Original ID and
// by its own snapshot ID, so that it can be matched against source snapshots.
func addDestinationSnapshot(dstSnapshotByOriginal map[restic.ID][]*data.Snapshot, sn *data.Snapshot) {
	if sn.Original != nil && !sn.Original.IsNull() {
		dstSnapshotByOriginal[*sn.Original] = append(dstSnapshotByOriginal[*sn.Original], sn)
	}
	// also consider identical snapshot copies
	dstSnapshotByOriginal[*sn.ID()] = append(dstSnapshotByOriginal[*sn.ID()], sn)
}

// findDestinationSnapshot returns the unique snapshot in the destination
// repository which corresponds to the given source snapshot. It fails if the
// corresponding snapshot is missing or exists more than once.
func findDestinationSnapshot(srcSn *data.Snapshot, dstSnapshotByOriginal map[restic.ID][]*data.Snapshot) (*data.Snapshot, error) {
	srcOriginal := *srcSn.ID()
	if srcSn.Original != nil {
		srcOriginal = *srcSn.Original
	}

	candidates, ok := dstSnapshotByOriginal[srcOriginal]
	if !ok {
		return nil, errors.Errorf("no corresponding snapshot found in the destination repository")
	}

	var dstSn *data.Snapshot
	for _, candidate := range candidates {
		if !similarSnapshots(candidate, srcSn) {
			continue
		}
		if dstSn != nil {
			return nil, errors.Errorf("multiple corresponding snapshots found in the destination repository (%s and %s)",
				dstSn.ID().Str(), candidate.ID().Str())
		}
		dstSn = candidate
	}

	if dstSn == nil {
		return nil, errors.Errorf("no corresponding snapshot found in the destination repository")
	}
	return dstSn, nil
}

// verifyTree recursively checks that the tree srcTreeID in the source
// repository and the tree dstTreeID in the destination repository contain the
// same nodes. All destination tree and data blobs are read, decrypted and
// decompressed, and their content hashes are verified. The prefix is the path
// of the tree within the snapshot, used for error messages.
func verifyTree(ctx context.Context, srcRepo restic.Repository, dstRepo restic.Repository,
	srcTreeID, dstTreeID restic.ID, prefix string, verifiedBlobs restic.AssociatedBlobSet) error {

	if err := ctx.Err(); err != nil {
		return err
	}

	dstTreeHandle := restic.BlobHandle{Type: restic.TreeBlob, ID: dstTreeID}
	// a shared tree that was already fully verified does not need to be checked again
	if srcTreeID.Equal(dstTreeID) && verifiedBlobs.Has(dstTreeHandle) {
		return nil
	}

	if _, ok := dstRepo.LookupBlobSize(dstTreeHandle); !ok {
		return errors.Errorf("path %q: tree blob %s is missing from the destination repository", prefix, dstTreeID.Str())
	}

	// loading the blob decrypts and decompresses it
	dstTreeData, err := dstRepo.LoadBlob(ctx, dstTreeHandle, nil)
	if err != nil {
		return fmt.Errorf("path %q: failed to load tree blob %s: %w", prefix, dstTreeID.Str(), err)
	}
	if hash := restic.Hash(dstTreeData); !hash.Equal(dstTreeID) {
		return errors.Errorf("path %q: tree blob %s content hash does not match, want %s, got %s",
			prefix, dstTreeID.Str(), dstTreeID.Str(), hash.Str())
	}

	dstTree, err := data.NewTreeNodeIterator(bytes.NewReader(dstTreeData))
	if err != nil {
		return fmt.Errorf("path %q: failed to parse tree blob %s: %w", prefix, dstTreeID.Str(), err)
	}
	srcTree, err := data.LoadTree(ctx, srcRepo, srcTreeID)
	if err != nil {
		return fmt.Errorf("path %q: failed to load source tree blob %s: %w", prefix, srcTreeID.Str(), err)
	}

	for dt := range data.DualTreeIterator(srcTree, dstTree) {
		if dt.Error != nil {
			return fmt.Errorf("path %q: failed to parse tree: %w", prefix, dt.Error)
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		srcNode, dstNode := dt.Tree1, dt.Tree2
		nodePath := prefix
		switch {
		case srcNode != nil:
			nodePath = path.Join(prefix, srcNode.Name)
		case dstNode != nil:
			nodePath = path.Join(prefix, dstNode.Name)
		}
		if srcNode == nil || dstNode == nil {
			side := "source"
			if dstNode == nil {
				side = "destination"
			}
			return errors.Errorf("path %q: node is missing in the %s repository", nodePath, side)
		}

		if err := verifyNode(ctx, srcRepo, dstRepo, srcNode, dstNode, nodePath, verifiedBlobs); err != nil {
			return err
		}
	}

	verifiedBlobs.Insert(dstTreeHandle)
	return nil
}

// verifyNode compares all relevant fields of the two corresponding nodes,
// verifies the node's content blobs in the destination repository and
// recursively checks the subtree of a directory node.
func verifyNode(ctx context.Context, srcRepo restic.Repository, dstRepo restic.Repository,
	srcNode, dstNode *data.Node, nodePath string, verifiedBlobs restic.AssociatedBlobSet) error {

	if err := compareNodes(srcNode, dstNode, nodePath); err != nil {
		return err
	}

	// verify the content blob sequence in the destination repository
	for _, blobID := range dstNode.Content {
		if err := verifyDataBlob(ctx, dstRepo, blobID, nodePath, verifiedBlobs); err != nil {
			return err
		}
	}

	// recursively verify directory subtrees
	if srcNode.Type == data.NodeTypeDir {
		if srcNode.Subtree == nil || dstNode.Subtree == nil {
			return errors.Errorf("path %q: subtree is missing", nodePath)
		}
		if err := verifyTree(ctx, srcRepo, dstRepo, *srcNode.Subtree, *dstNode.Subtree, nodePath, verifiedBlobs); err != nil {
			return err
		}
	}
	return nil
}

// compareNodes checks that the node path, type, size, mode, times, link
// target, device information, content blob sequence, extended attributes,
// generic attributes and subtree are identical in both nodes.
func compareNodes(srcNode, dstNode *data.Node, nodePath string) error {
	mismatch := func(field string, want, got any) error {
		return errors.Errorf("path %q: %s mismatch: source=%v, destination=%v", nodePath, field, want, got)
	}

	if srcNode.Name != dstNode.Name {
		return mismatch("name", srcNode.Name, dstNode.Name)
	}
	if srcNode.Type != dstNode.Type {
		return mismatch("node type", srcNode.Type, dstNode.Type)
	}
	if srcNode.Mode != dstNode.Mode {
		return mismatch("mode", srcNode.Mode, dstNode.Mode)
	}
	if srcNode.Size != dstNode.Size {
		return mismatch("size", srcNode.Size, dstNode.Size)
	}
	if !srcNode.ModTime.Equal(dstNode.ModTime) {
		return mismatch("modification time", srcNode.ModTime, dstNode.ModTime)
	}
	if !srcNode.AccessTime.Equal(dstNode.AccessTime) {
		return mismatch("access time", srcNode.AccessTime, dstNode.AccessTime)
	}
	if !srcNode.ChangeTime.Equal(dstNode.ChangeTime) {
		return mismatch("change time", srcNode.ChangeTime, dstNode.ChangeTime)
	}
	if srcNode.UID != dstNode.UID {
		return mismatch("uid", srcNode.UID, dstNode.UID)
	}
	if srcNode.GID != dstNode.GID {
		return mismatch("gid", srcNode.GID, dstNode.GID)
	}
	if srcNode.User != dstNode.User {
		return mismatch("user", srcNode.User, dstNode.User)
	}
	if srcNode.Group != dstNode.Group {
		return mismatch("group", srcNode.Group, dstNode.Group)
	}
	if srcNode.Inode != dstNode.Inode {
		return mismatch("inode", srcNode.Inode, dstNode.Inode)
	}
	if srcNode.DeviceID != dstNode.DeviceID {
		return mismatch("device id", srcNode.DeviceID, dstNode.DeviceID)
	}
	if srcNode.Links != dstNode.Links {
		return mismatch("hardlink count", srcNode.Links, dstNode.Links)
	}
	if srcNode.LinkTarget != dstNode.LinkTarget {
		return mismatch("link target", srcNode.LinkTarget, dstNode.LinkTarget)
	}
	if srcNode.Device != dstNode.Device {
		return mismatch("device", srcNode.Device, dstNode.Device)
	}
	if len(srcNode.Content) != len(dstNode.Content) {
		return errors.Errorf("path %q: content blob sequence length mismatch: source=%d, destination=%d",
			nodePath, len(srcNode.Content), len(dstNode.Content))
	}
	for i := range srcNode.Content {
		if !srcNode.Content[i].Equal(dstNode.Content[i]) {
			return errors.Errorf("path %q: content blob at position %d mismatch: source=%s, destination=%s",
				nodePath, i, srcNode.Content[i].Str(), dstNode.Content[i].Str())
		}
	}
	if !reflect.DeepEqual(srcNode.ExtendedAttributes, dstNode.ExtendedAttributes) {
		return mismatch("extended attributes", srcNode.ExtendedAttributes, dstNode.ExtendedAttributes)
	}
	if !reflect.DeepEqual(srcNode.GenericAttributes, dstNode.GenericAttributes) {
		return mismatch("generic attributes", srcNode.GenericAttributes, dstNode.GenericAttributes)
	}
	switch {
	case srcNode.Subtree == nil && dstNode.Subtree != nil:
		return errors.Errorf("path %q: subtree is missing in the source repository", nodePath)
	case srcNode.Subtree != nil && dstNode.Subtree == nil:
		return errors.Errorf("path %q: subtree is missing in the destination repository", nodePath)
	case srcNode.Subtree != nil && dstNode.Subtree != nil && !srcNode.Subtree.Equal(*dstNode.Subtree):
		return errors.Errorf("path %q: subtree mismatch: source=%s, destination=%s",
			nodePath, srcNode.Subtree.Str(), dstNode.Subtree.Str())
	}
	return nil
}

// verifyDataBlob checks that the data blob exists in the destination
// repository and that the hash of its decrypted and decompressed content
// matches its blob ID.
func verifyDataBlob(ctx context.Context, dstRepo restic.Repository, blobID restic.ID,
	nodePath string, verifiedBlobs restic.AssociatedBlobSet) error {

	handle := restic.BlobHandle{Type: restic.DataBlob, ID: blobID}
	if _, ok := dstRepo.LookupBlobSize(handle); !ok {
		return errors.Errorf("path %q: data blob %s is missing from the destination repository", nodePath, blobID.Str())
	}
	if verifiedBlobs.Has(handle) {
		return nil
	}

	// loading the blob decrypts and decompresses it
	buf, err := dstRepo.LoadBlob(ctx, handle, nil)
	if err != nil {
		return fmt.Errorf("path %q: failed to load data blob %s: %w", nodePath, blobID.Str(), err)
	}
	if hash := restic.Hash(buf); !hash.Equal(blobID) {
		return errors.Errorf("path %q: data blob %s content hash does not match, want %s, got %s",
			nodePath, blobID.Str(), blobID.Str(), hash.Str())
	}
	verifiedBlobs.Insert(handle)
	return nil
}
