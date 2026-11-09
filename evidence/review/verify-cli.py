import hashlib,json,os,pathlib,subprocess,sys,tempfile
binary=pathlib.Path(sys.argv[1]); root=pathlib.Path(tempfile.mkdtemp(prefix='cli-',dir=sys.argv[2]))
(root/'input/sub').mkdir(parents=True)
(root/'src.pass').write_text('source-review-key\n');(root/'dst.pass').write_text('destination-review-key\n')
(root/'input/alpha').write_bytes(b'cross-key backup\n'*1000)
(root/'input/sub/beta').write_bytes(bytes(range(256))*40)
os.symlink('../alpha',root/'input/sub/link')
records=[]
def call(repo,*args,expected=0):
 cmd=[str(binary),'--no-cache','-r',str(root/repo),'-p',str(root/('src.pass' if repo=='src' else 'dst.pass')),*map(str,args)]
 p=subprocess.run(cmd,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 records.append({'command':cmd,'exitCode':p.returncode,'output':p.stdout})
 print('CLI',json.dumps(list(map(str,args))),'EXIT',p.returncode,flush=True);print(p.stdout,flush=True)
 assert p.returncode==expected,(cmd,p.stdout)
 return p.stdout
for repo in ['src','dst']:call(repo,'init')
call('src','backup','--tag','selected',root/'input')
call('src','backup','--tag','excluded',root/'input')
copy=['copy','--from-repo',root/'src','--from-password-file',root/'src.pass','--tag','selected']
fresh=call('dst',*copy,'--verify');assert 'verified 1 snapshots' in fresh
skip=call('dst',*copy,'--verify');assert 'verified 1 snapshots' in skip
sn=json.loads(call('dst','snapshots','--json'));assert len(sn)==1
call('dst','check','--read-data')
call('dst','restore','latest','--target',root/'restore')
for p in (root/'input').rglob('*'):
 target=root/'restore'/p.relative_to('/')
 if p.is_symlink():assert os.readlink(p)==os.readlink(target)
 elif p.is_file():assert hashlib.sha256(p.read_bytes()).digest()==hashlib.sha256(target.read_bytes()).digest()
print('RESTORE_BYTE_AND_SYMLINK_COMPARISON PASS',flush=True)
# Default copy retains filtering and skip; no verification output is emitted.
normal=call('dst',*copy);assert 'verified' not in normal
assert len(json.loads(call('dst','snapshots','--json')))==1
call('dst','copy','--from-repo',root/'src','--from-password-file',root/'src.pass','--verify','not-a-snapshot',expected=1)
assert not list((root/'src/locks').iterdir()) and not list((root/'dst/locks').iterdir())
# Damage a real encrypted pack. The index still names it, so verify must read it.
packs=[p for p in (root/'dst/data').rglob('*') if p.is_file()]
pack=packs[0];pack.chmod(0o600);raw=bytearray(pack.read_bytes());raw[len(raw)//2]^=0xff;pack.write_bytes(raw)
bad=call('dst',*copy,'--verify',expected=1)
assert 'snapshot' in bad and 'path' in bad and 'verified' not in bad
assert not list((root/'src/locks').iterdir()) and not list((root/'dst/locks').iterdir())
print('CLI_LOCKS source=0 destination=0; CORRUPT_ENCRYPTED_PACK_REJECTED PASS',flush=True)
out=pathlib.Path(__file__).parent
(out/'cli-result.json').write_text(json.dumps({'records':records,'restoreCompared':True,'locksReleased':True},ensure_ascii=False,indent=2)+'\n')
