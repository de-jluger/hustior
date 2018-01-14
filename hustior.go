/*
   Copyright (C) 2017-2018  J. Luger

   This file is part of hustior.

   hustior is free software: you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   hustior is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.

   You should have received a copy of the GNU General Public License
   along with Foobar.  If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path"
	"strconv"
	"syscall"
)

func main() {
	userJson := flag.String("user", "", "")
	flag.Parse()
	if userJson == nil || *userJson == "" {
		restartInNamespace()
		return
	}
	user := user.User{}
	err := json.Unmarshal([]byte(*userJson), &user)
	if err != nil {
		log.Println("Wrong user provided. Please don't use the argument manually")
		return
	}
	rootBase := setUpNewRootFS()
	setUpHomeDirectory(user, rootBase)
	startCommand(rootBase, user)
}

//Restarts this application in a new user, mount and pid namespace while the user id
//of the caller is mapped to 0.
func restartInNamespace() {
	user, err := user.Current()
	onErrorLogAndExit(err)
	if user.HomeDir == "/root" {
		log.Println("Don't call as root")
		return
	}
	userData, err := json.Marshal(user)
	onErrorLogAndExit(err)
	cmd := exec.Command(os.Args[0], append([]string{"-user", string(userData)}, flag.Args()...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getgid(),
				Size:        1,
			},
		},
	}
	onErrorLogAndExit(cmd.Run())
}

//Sets up a new root filesystem that the sandbox should use.
//The return value is the location of the new root.
func setUpNewRootFS() (rootBase string) {
	err := syscall.Mount("none", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, "")
	onErrorLogAndExit(err)
	err = syscall.Mount("none", "/root", "tmpfs", 0, "size=200M")
	onErrorLogAndExit(err)
	rootBase = "/root/base"
	devDir := rootBase + "/dev"
	procDir := rootBase + "/proc"
	createDirs := []string{rootBase, devDir, rootBase + "/run", procDir, rootBase + "/tmp"}
	for _, dir := range createDirs {
		onErrorLangAndExitWithDesc(syscall.Mkdir(dir, 0755), dir)
	}
	bindDirs := []string{"/bin", "/etc", "/lib", "/lib64", "/opt", "/sbin", "/usr", "/var", "/dev/shm", "/run/resolvconf", "/run/user"}
	for _, dir := range bindDirs {
		path := rootBase + dir
		onErrorLangAndExitWithDesc(syscall.Mkdir(path, 0755), path)
		onErrorLangAndExitWithDesc(syscall.Mount(dir, path, "", syscall.MS_REC|syscall.MS_BIND, ""), path)
	}
	devFiles := []string{"random", "urandom", "null", "zero"}
	for _, devFile := range devFiles {
		onErrorLangAndExitWithDesc(mountBindDevDir(devDir, devFile), "/dev/"+devFile)
	}
	//The mount of /proc is currently necessary or I will get a "fork/exec /bin/bash: permission denied"
	err = syscall.Mount("proc", "/proc", "proc", syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NODEV, "")
	onErrorLogAndExit(err)
	err = syscall.Mount("proc", procDir, "proc", syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NODEV, "")
	onErrorLogAndExit(err)
	return
}

//Takes the unnamed arguments as directories that are bound under <rootBase>/home/<user>/
func setUpHomeDirectory(user user.User, rootBase string) {
	newHomeDir := rootBase + user.HomeDir
	onErrorLogAndExit(os.MkdirAll(newHomeDir, 0700))
	for _, p := range flag.Args() {
		dirName := path.Base(p)
		absDirName := newHomeDir + "/" + dirName
		onErrorLogAndExit(syscall.Mkdir(absDirName, 0700))
		onErrorLogAndExit(syscall.Mount(p, absDirName, "", syscall.MS_BIND|syscall.MS_REC, ""))
	}
	return
}

//Starts a bash. The bash runs in a new user namespace where the root id is mapped back to the original user id and
//the bash is chrooted in the given rootBase.
func startCommand(rootBase string, user user.User) {
	uid, err := strconv.Atoi(user.Uid)
	onErrorLogAndExit(err)
	gid, err := strconv.Atoi(user.Gid)
	onErrorLogAndExit(err)
	cmd := exec.Command("/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = user.HomeDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot:     rootBase,
		Cloneflags: syscall.CLONE_NEWUSER,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: uid,
				HostID:      os.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: gid,
				HostID:      os.Getgid(),
				Size:        1,
			},
		},
	}
	onErrorLogAndExit(cmd.Run())
}

//Creates the given device file in the given dev dir and then bind the device file from /dev to it.
func mountBindDevDir(devDir, deviceName string) error {
	devicePath := devDir + "/" + deviceName
	deviceFile, err := os.OpenFile(devicePath, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	deviceFile.Close()
	err = syscall.Mount("/dev/"+deviceName, devicePath, "", syscall.MS_BIND, "")
	return err
}

// When the given error is not nil then print it and exit the appication.
func onErrorLogAndExit(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

// When the given error is not nil then print it together with the description and exit the appication.
func onErrorLangAndExitWithDesc(e error, description string) {
	if e != nil {
		log.Fatal(description, e)
	}
}
