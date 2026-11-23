#!/usr/bin/env python3
"""Rebuild bundled preparation-only DBOPL helpers from their supplied source.

Developer/build operation only. Normal asset preparation extracts an embedded
native helper and never downloads a compiler or runs this script.
"""
from pathlib import Path
import argparse
import gzip
import hashlib
import json
import os
import shutil
import struct
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[1]
PKG = ROOT/'src/internal/audioopl'
SOURCE = PKG/'source'
REVISION = 'f95fb588686e81fdcfb4c31149addcc18ff64866'
UPSTREAM = {
 'dbopl.cpp': 'f25fd1ad953c2f6a8ba4030d0ae583dfb76b5e6b3212e36b24d712ae63635f07',
 'dbopl.h': '63985cee9a04d77a7b9bcde35d77b931c0a7558019655632ab6df213453d0159',
}
TARGETS = {
 'linux-amd64': ('x86_64-linux-musl', ''),
 'windows-amd64': ('x86_64-windows-gnu', '.exe'),
 'linux-arm64': ('aarch64-linux-musl', ''),
}

def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()

def validate_source():
    for name,digest in UPSTREAM.items():
        if sha(SOURCE/'upstream'/name)!=digest:
            raise ValueError('Pinned upstream source changed: '+name)
    cpp=(SOURCE/'upstream/dbopl.cpp').read_bytes()
    h=(SOURCE/'upstream/dbopl.h').read_bytes()
    expected_cpp=b'// Modified 2026-10-23: retain DBOPL Chip core; omit DOSBox mixer/serialization Handler.\n'+cpp[:cpp.index(b'uint32_t Handler::WriteAddr(')]+b'\n} // namespace DBOPL; preparation-only adapter stops before DOSBox Handler\n'
    expected_h=b'// Modified 2026-10-23: retain synthesis types; replace DOSBox adapter dependency with local types.\n'+h[:h.index(b'struct Handler :')].replace(b'#include "adlib.h"',b'// Preparation adapter: DOSBox mixer/header dependency excluded.\n#include <cstdint>')+b'\n} // namespace DBOPL\n'
    if (SOURCE/'dbopl.cpp').read_bytes()!=expected_cpp or (SOURCE/'dbopl.h').read_bytes()!=expected_h:
        raise ValueError('Core differs from the recorded adapter-only trim; review and version the component')

def make_runtime_source_archive(zig, destination):
    # Retain actual sources/notices for the static runtime components as well as
    # the DBOPL converter. The compiler itself remains a pinned build prerequisite.
    home=zig.parent
    selected=['LICENSE','lib/libcxx','lib/libcxxabi','lib/libunwind','lib/compiler_rt.zig','lib/compiler_rt','lib/std','lib/libc/musl','lib/libc/mingw','lib/libc/include/generic-musl','lib/libc/include/x86_64-linux-musl','lib/libc/include/aarch64-linux-musl','lib/libc/include/any-windows-any','lib/include']
    with destination.open('wb') as raw:
        with gzip.GzipFile(fileobj=raw,mode='wb',mtime=0,filename='') as zipped:
            with tarfile.open(fileobj=zipped,mode='w') as archive:
                for name in selected:
                    p=home/name
                    if not p.exists():raise ValueError('Expected pinned Zig runtime source absent: '+str(p))
                    entries=[p]+sorted(p.rglob('*')) if p.is_dir() else [p]
                    for item in entries:
                        info=archive.gettarinfo(str(item),arcname='zig-0.13.0-runtime-sources/'+item.relative_to(home).as_posix())
                        info.uid=info.gid=0;info.uname=info.gname='';info.mtime=0
                        if item.is_file():
                            with item.open('rb') as f:archive.addfile(info,f)
                        else:archive.addfile(info)

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--zig',type=Path,required=True)
    p.add_argument('--output',type=Path,required=True)
    p.add_argument('--embed',action='store_true',help='Update only the generated embedded helper payloads/manifest in src/internal/audioopl')
    a=p.parse_args();a.zig=a.zig.resolve();out=a.output.resolve()
    if out.exists():p.error('Build output must not exist')
    version=subprocess.check_output([str(a.zig),'version'],text=True).strip()
    if version!='0.13.0':p.error('Pinned build requires Zig0.13.0; got '+version)
    validate_source();out.mkdir(parents=True)
    env=dict(os.environ,ZIG_GLOBAL_CACHE_DIR=str(out/'zig-cache'),SOURCE_DATE_EPOCH='0')
    source_hashes={x.relative_to(SOURCE).as_posix():sha(x) for x in sorted(SOURCE.rglob('*')) if x.is_file() and (x.suffix in ('.cpp','.h') or x.name=='COPYING')}
    m=dict(name='DOSBox-X DBOPL preparation and flight helper',source_revision=REVISION,source_sha256=source_hashes,license='GPL-2.0-or-later',protocol_version=1,stream_protocol_version=1,helpers={},
           synthesis_policy='Unmodified retained DBOPL Chip; OPL3-capable setup/mono OPL2-compatible writes; raw signed32LE; no gain/normalization',
           upstream_url='https://github.com/joncampbell123/dosbox-x/tree/'+REVISION,
           adapter='Trim DOSBox Handler/mixer/save-state adapters only; retain exact upstream bytes; minimal integer-type shim; standalone bounded stdin/stdout CLI',
           build_tool=dict(name='Zig',version=version,distribution_sha256='d45312e61ebcc48032b77bc4cf7fd6915c11fa16e4aad116b66c9468211230ea',url='https://ziglang.org/download/0.13.0/zig-linux-x86_64-0.13.0.tar.xz'))
    for platform,(target,extension) in TARGETS.items():
        binary=out/('audio-bake-'+platform+extension)
        command=[str(a.zig),'c++','-target',target,'-O2','-std=c++17','-static','-s','-I',str(SOURCE),str(SOURCE/'dbopl.cpp'),str(SOURCE/'helper.cpp'),'-o',str(binary)]
        result=subprocess.run(command,env=dict(env,ZIG_LOCAL_CACHE_DIR=str(out/('cache-'+platform))),capture_output=True,text=True)
        (out/(platform+'.log')).write_text(result.stdout+result.stderr)
        if result.returncode:raise RuntimeError('Helper build failed: '+platform+'; see '+str(out/(platform+'.log')))
        pre_normalization_sha=sha(binary)
        if platform.startswith('windows'):
            # This is our unsigned build artifact, not an original game binary.
            # Zig0.13 inserts a PE COFF timestamp; zero only that metadata field.
            image=bytearray(binary.read_bytes());pe=struct.unpack_from('<I',image,0x3c)[0]
            if image[pe:pe+4]!=b'PE\0\0':raise ValueError('Expected PE helper')
            struct.pack_into('<I',image,pe+8,0);binary.write_bytes(image)
        compressed=gzip.compress(binary.read_bytes(),mtime=0)
        gz=out/('audio-bake-'+platform+'.gz');gz.write_bytes(compressed)
        m['helpers'][platform]=dict(sha256=sha(binary),bytes=binary.stat().st_size,compressed_sha256=sha(gz),compressed_bytes=len(compressed),target=target,
                                   pre_normalization_sha256=pre_normalization_sha,normalization='PE COFF timestamp only zeroed' if platform.startswith('windows') else 'none',requirements='Windows10+ UCRT/KERNEL32 system components' if platform.startswith('windows') else 'Static musl executable; no shared C++/libc runtime required')
        if a.embed:
            (PKG/'bin').mkdir(exist_ok=True)
            shutil.copyfile(gz,PKG/'bin'/gz.name)
            suffix=platform.replace('-','_')
            (PKG/('payload_'+suffix+'.go')).write_text('package audioopl\n\nimport _ "embed"\n\n//go:embed bin/'+gz.name+'\nvar helperCompressed []byte\n\nfunc nativePayload() ([]byte,error) { return helperCompressed,nil }\n')
        print(platform,sha(binary),flush=True)
    archive=out/'audio-helper-runtime-sources.tar.gz'
    make_runtime_source_archive(a.zig,archive)
    m['runtime_sources']=dict(path=archive.name,sha256=sha(archive),bytes=archive.stat().st_size,
                             contents='Zig0.13 LLVM C++/ABI/unwind, compiler-rt/std, musl and MinGW runtime sources, interfaces and notices')
    (out/'component.json').write_text(json.dumps(m,indent=2)+'\n')
    if a.embed:
        shutil.copyfile(out/'component.json',PKG/'component.json')
        shutil.copyfile(archive,PKG/'source'/'audio-helper-runtime-sources.tar.gz')
    print('Complete source/runtime notices must accompany distribution. No Windows/ARM execution claim.',flush=True)

if __name__=='__main__':main()
