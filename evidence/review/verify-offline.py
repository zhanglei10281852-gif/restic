import datetime, hashlib, json, os, pathlib, socket, subprocess, sys, tarfile, tempfile

repo = pathlib.Path(__file__).resolve().parents[2]
out = repo / 'evidence/review'
materials = repo.parent / 'authoring/offline-materials'
if len(sys.argv) == 1:
    manifest = json.loads((materials / 'manifest.json').read_text())
    for item in manifest:
        assert hashlib.sha256((materials / item['file']).read_bytes()).hexdigest() == item['sha256'], item['file']
    run = pathlib.Path(tempfile.mkdtemp(prefix='restic-review-'))
    run.chmod(0o755)
    with tarfile.open(materials / 'go1.26.10-linux-amd64.tar.gz') as tf:
        tf.extractall(run, filter='data')
    with (out / 'runtime.log').open('w') as log:
        p = subprocess.run(['unshare', '-n', 'python3', str(pathlib.Path(__file__).resolve()), str(run)], stdout=log, stderr=subprocess.STDOUT)
    print(json.dumps({'exitCode': p.returncode, 'run': str(run), 'log': str(out/'runtime.log')}))
    sys.exit(p.returncode)

run = pathlib.Path(sys.argv[1])
for name in ['modcache', 'buildcache', 'home', 'tmp', 'runtime-cache']:
    p = run / name
    p.mkdir(exist_ok=True)
    os.chown(p, 65534, 65534)
env = dict(os.environ, HOME=str(run/'home'), GOTOOLCHAIN='local', GOWORK='off', GOMAXPROCS='4',
    GOMODCACHE=str(run/'modcache'), GOCACHE=str(run/'buildcache'), GOPROXY=materials.joinpath('proxy').as_uri(),
    GOSUMDB='off', CGO_ENABLED='0', RESTIC_CACHE_DIR=str(run/'runtime-cache'), TMPDIR=str(run/'tmp'))
go = str(run/'go/bin/go')
results = []
def demote():
    os.setgroups([])
    os.setgid(65534)
    os.setuid(65534)
def command(args, expected=0, uid=True):
    args = list(map(str, args))
    print('COMMAND', json.dumps(args), 'UID', 65534 if uid else 0, flush=True)
    p = subprocess.run(args, cwd=repo, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, preexec_fn=demote if uid else None)
    print(p.stdout, flush=True)
    print('EXIT', p.returncode, flush=True)
    results.append({'command': args, 'exitCode': p.returncode, 'output': p.stdout})
    if expected is not None:
        assert p.returncode == expected
    return p
print('MANIFEST_VERIFIED', len(json.loads((materials/'manifest.json').read_text())), flush=True)
print('ENVIRONMENT', json.dumps({k:env[k] for k in ['HOME','GOMODCACHE','GOCACHE','GOPROXY','RESTIC_CACHE_DIR','CGO_ENABLED']}), flush=True)
command([sys.executable, '-c', "import socket,os;print('UID',os.getuid());socket.create_connection(('1.1.1.1',443),timeout=1)"], expected=1)
command([go, 'version'])
if len(sys.argv)==2:
    command([go, 'test', './cmd/restic', '-run', '^(TestCopy|TestFindDestinationSnapshot$|TestCompareNodes$|TestVerifyTreeDetectsTamperedBlobs$)', '-count=1', '-v'])
    overlay = run/'overlay.json'
    overlay.write_text(json.dumps({'Replace': {str(repo/'cmd/restic/review_contract_test.go'):str(out/'review_contract_test.go.txt')}}))
    command([go, 'test', '-overlay='+str(overlay), './cmd/restic', '-run', '^TestReview', '-count=3', '-v'])
    command([go, 'build', '-buildvcs=false', '-o', run/'tmp/restic', './cmd/restic'])
command([sys.executable,out/'verify-cli.py',run/'tmp/restic',run/'tmp'])
(out/'runtime-result.json').write_text(json.dumps({'run':str(run),'materials':str(materials),'results':results}, ensure_ascii=False, indent=2)+'\n')
print('OFFLINE_REVIEW_INITIAL_COMPLETED', flush=True)
