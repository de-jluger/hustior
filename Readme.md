# Table of Contents
1. [How to build](#how-to-build)
2. [What does it do](#What-does-it-do)
3. [When to use](#third-example)
4. [When not to use](#when-not-to-use)
5. [Limits](#limits)


## How to build
go build hustior.go


## What does it do?
hustior tries to hide all of your home directory except the folders you specify as the program arguments and it will hide all other processes.  
If you call hustior with no arguments it will start a bash and you will be in your home directory. But your home directory will be empty. Don't worry your file won't be deleted. They are just not visible from this bash (and all their child processes).  
When you call hustior with the argument "/home/<user>/path1/path2" it will be the same as above except that you have one directory in your home: "path2". "path2" will contain all the subdirectories and files that it also contains outside of the hustior started bash.  
When stopping the hustior started bash all files saved in the home will disappear. Except they were stored in a mounted directory. Like e.g. "path2" from the example above.
With the parameter "-exec" you can pass a program name that the bash will execute as a command. This allows you also to add parameters to the program. As a downside the program needs to be blocking or the bash will end and with it the desired program.  
With the parameter "-configFile" you can specify a file that contains one or all of the previous mentioned configurations. Use "-printConfigSample" to get a sample.  
I've added an additional parameter "AdditionalBindings" that is only available via a config file. With that you can specify additional files and folders that should be visible in the container.

## When to use
Use it when you want to get an extra line of defense before executing software or surfing on a site when everything should be OK. E.g. you start software like NodeJS, VisualCode, Eclipse, ... All this software is created by well known companies/organisations and used by millions of people. It should be safe to use. But one malicious/infected plugin and an attacker has access to all your mails/picutures/videos/documents. See https://xkcd.com/1200/.  
The idea of hustior is to allow programs like Eclipse only access to its binary and its workspace. There is no need to access my mails or my holiday pictures.

## When not to use
hustior is not battle hardened. Do not use it when you expect to get attacked. E.g. you execute a script/binary downloaded from a shady website.  
In this case use a virtual machine on a spare PC that you nuke regularly.  
Also don't call hustior as root. You should have absolutely no doubt about the software you execute as root.

## Limits
The temp filesystem used is only 200MB. Storing more in your home isn't possible. When you want to store larger files give a directory as an argument and store the files there.  
The started programs have full network access and can even access ports opened by other processes.  
The processes started by the hustior bash aren't isolated from attacks outside. E.g. an attacker made it to your machine and runs in your user context (not restricted by hustior) and you start a browser from the hustior bash. The attacker can see the browser.  
Setuid won't work. As a normal user you can only map your own user id. Per default "an unmapped user ID is converted to the overflow user ID"(http://man7.org/linux/man-pages/man7/user_namespaces.7.html). So in the restarted hustior all files that belonged to root will belong to nobody (when nobody has the overflow user ID). I haven't found a way to fix this when starting the bash. So in the bash the setuid programs like ping or Chrome sandbox won't start as the don't belong to root.  
Please also note that hustior is currently tested only on Ubuntu 17.10.
