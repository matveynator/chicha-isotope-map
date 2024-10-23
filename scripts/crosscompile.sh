#!/bin/bashi
version="1.0-012"
git_root_path=`git rev-parse --show-toplevel`
execution_file="isotope-pathways"

go mod download
go mod tidy

echo "Performing tests on all modules..."
go test ./...
if [ $? != "0" ] 
then
  echo "Tests on all modules failed."
  echo "Press any key to continue compilation or CTRL+C to abort."
  read
else 
  echo "Tests on all modules passed."
fi

cd ${git_root_path}/scripts;

mkdir -p ${git_root_path}/binaries/${version};

rm -f ${git_root_path}/binaries/latest; 

cd ${git_root_path}/binaries; ln -s ${version} latest; cd ${git_root_path}/scripts;

for os in android aix darwin dragonfly freebsd illumos ios js linux netbsd openbsd plan9 solaris windows zos;
#for os in windows;
do
  for arch in "amd64" "386" "arm" "arm64" "mips64" "mips64le" "mips" "mipsle" "ppc64" "ppc64le" "riscv64" "s390x" "wasm" 
  do
    target_os_name=${os}
    [ "$os" == "windows" ] && execution_file="isotope-pathways.exe" && build_flags="-H windowsgui "
    [ "$os" == "darwin" ] && target_os_name="mac"

    #compile non gui app:
    GOOS=${os} GOARCH=${arch} go build -ldflags "-X isotope-pathways/pkg/config.CompileVersion=${version}" -o ${execution_file} ../isotope-pathways.go 2> /dev/null

    if [ "$?" == "0" ]
    then
      mkdir -p ../binaries/${version}/no-gui/${target_os_name}/${arch}
      chmod +x ${execution_file}
      mv ${execution_file} ../binaries/${version}/no-gui/${target_os_name}/${arch}/
      echo "GOOS=${os} GOARCH=${arch} go build -ldflags "-X isotope-pathways/pkg/config.CompileVersion=${version}" -o ../binaries/${version}/no-gui/${target_os_name}/${arch}/${execution_file} ../isotope-pathways.go"
    fi

  done
done

#optional: publish to internet:
rsync -avP ../binaries/* files@files.zabiyaka.net:/home/files/public_html/isotope-pathways/
