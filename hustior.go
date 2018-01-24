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

//Stores the configuration of the program.
//This is needed to easily pass program configration to the children created after
//the original invocation through the user.
type programConfig struct {
	ExecProgramm    string
	HomeDirectories []string
}

func main() {
	pg, userJSON := parseArgs()
	if userJSON == "" {
		restartInNamespace(pg)
		return
	}
	user := user.User{}
	err := json.Unmarshal([]byte(userJSON), &user)
	if err != nil {
		log.Println("Wrong user provided. Please don't use the argument manually")
		return
	}
	rootBase := setUpNewRootFS()
	setUpHomeDirectory(rootBase, user, pg.HomeDirectories)
	startCommand(rootBase, user, pg.ExecProgramm)
}

//Parses the program arguments for the programConfig and the user information.
//The user information is either an emtpy string (tpyicall on invocation through the user)
//or the JSON version of the user.User instance of the original caller.
//programConfig is always there but its field may be empty.
func parseArgs() (programConfig, string) {
	var pg programConfig
	userJSON := flag.String("user", "", "")
	execProgramm := flag.String("exec", "", "")
	confJSON := flag.String("config", "", "")
	flag.Parse()
	if confJSON != nil && *confJSON != "" {
		err := json.Unmarshal([]byte(*confJSON), &pg)
		if err != nil {
			onErrorLogAndExit(err)
		}
	}
	if execProgramm != nil && *execProgramm != "" {
		pg.ExecProgramm = *execProgramm
	}
	homeDirectories := []string{}
	for _, p := range flag.Args() {
		homeDirectories = append(homeDirectories, p)
	}
	if len(homeDirectories) > 0 || pg.HomeDirectories == nil {
		pg.HomeDirectories = homeDirectories
	}
	return pg, *userJSON
}

//Restarts this application in a new user, mount and pid namespace while the user id
//of the caller is mapped to 0.
//The given programConfig will be passed to the newly created child process.
func restartInNamespace(pg programConfig) {
	user, err := user.Current()
	onErrorLogAndExit(err)
	if user.HomeDir == "/root" {
		log.Println("Don't call as root")
		return
	}
	userData, err := json.Marshal(user)
	onErrorLogAndExit(err)
	confData, err := json.Marshal(pg)
	onErrorLogAndExit(err)
	cmd := exec.Command(os.Args[0], "-user", string(userData), "-config", string(confData))
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

//Takes the strings in homeDirectories as directories that are bound under <rootBase>/home/<user>/
func setUpHomeDirectory(rootBase string, user user.User, homeDirectories []string) {
	newHomeDir := rootBase + user.HomeDir
	onErrorLogAndExit(os.MkdirAll(newHomeDir, 0700))
	for _, hd := range homeDirectories {
		dirName := path.Base(hd)
		absDirName := newHomeDir + "/" + dirName
		onErrorLogAndExit(syscall.Mkdir(absDirName, 0700))
		onErrorLogAndExit(syscall.Mount(hd, absDirName, "", syscall.MS_BIND|syscall.MS_REC, ""))
	}
	return
}

//Starts a bash. The bash runs in a new user namespace where the root id is mapped back to the original user id and
//the bash is chrooted in the given rootBase. When the given execProgramm string is not empty it will be passed as
//a command to the bash (so there will always be a bash for program reaping)
func startCommand(rootBase string, user user.User, execProgramm string) {
	uid, err := strconv.Atoi(user.Uid)
	onErrorLogAndExit(err)
	gid, err := strconv.Atoi(user.Gid)
	onErrorLogAndExit(err)
	command := "/bin/bash"
	var cmd *exec.Cmd
	if execProgramm != "" {
		cmd = exec.Command(command, "-c", execProgramm)
	} else {
		cmd = exec.Command(command)
	}
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
