package main

import (
	"net/http"
	"io/ioutil"
	"regexp"
	"os"
	"strings"
	"fmt"
	"os/exec"
	"path/filepath"
	"bufio"
	"syscall"
	"time"
	"math/rand"
	"strconv"
	"github.com/kardianos/osext"

)

const imgroot  = "/var/lib/libvirt/images/"

func findcrpm(ver string) string {


	cURL:="http://mirrors.sohu.com/centos/" + ver + "/os/x86_64/Packages/"
	resp,_:=http.Get(cURL)
	defer resp.Body.Close()
	a,_:=ioutil.ReadAll(resp.Body)
	reg:=regexp.MustCompile(`centos-release.*.x86_64.rpm"`)
	s:=reg.Find(a)
	if len(s) == 0 {return ""}
	return cURL+string([]rune(string(s))[:len(s)-1])

}


type shell struct {
	*exec.Cmd
}

func runCmd(cmd string,args ...string) *shell  {
	return &shell{exec.Command(cmd,args...)}

}

func (s *shell) do() error {
	return s.Run()
}
func (s *shell) stdio() *shell {
	s.Stdout=os.Stdout
	s.Stdin=os.Stdin
	s.Stderr=os.Stderr
	return s
}

func fileWrite(path,content string) (err error) {
	fi,err:=os.Create(path)
	if err != nil {return err}
	defer fi.Close()
	wr:=bufio.NewWriter(fi)
	_,err=wr.WriteString(content)
	err=wr.Flush()
	err=fi.Close()

	return nil

}

func pull(name ,ver string)  {


	tmp,_:=ioutil.TempDir("/tmp","kyum")
	os.Chdir(tmp)

	if e:=runCmd("rpm","-ivh","--root="+tmp,findcrpm(ver)).do();e!=nil{

		os.RemoveAll(tmp)

		fmt.Println("error:pull fail")
		return
	}

fstab:=`
LABEL=/                 /                       ext4    defaults        0 0
tmpfs                   /dev/shm                tmpfs   defaults        0 0
devpts                  /dev/pts                devpts  gid=5,mode=620  0 0
sysfs                   /sys                    sysfs   defaults        0 0
proc                    /proc                   proc    defaults        0 0`
	fileWrite(filepath.Join(tmp,"etc/fstab"),fstab)
	fileWrite(filepath.Join(tmp,"etc/sysconfig/network"),"")


	if e:=runCmd("yum","install","-y","--nogpgcheck","--installroot="+tmp,"kernel","epel-release").stdio().do();e!=nil{

		os.RemoveAll(tmp)
		fmt.Println("error:install fail")

		return
	}



	if e:=runCmd("tar","-jcf",filepath.Join(imgroot,name),".").stdio().do();e!=nil{
		os.RemoveAll(filepath.Join(imgroot,name))
		fmt.Println("error:pack fail")
		return
	}

	//runCmd("umount",tmp).do()
	os.RemoveAll(tmp)
	fmt.Println("pull complete.")



}

func monitm(imgpath string)  {



	if os.Getppid() != 1 {
		abspath, _ := osext.Executable()
		attr := &os.ProcAttr{
			Dir:"/",
			Env:os.Environ(),
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
			Sys: &syscall.SysProcAttr{
				//Chroot:     d.Chroot,
				Credential: &syscall.Credential{Uid:uint32(os.Getuid())},
				Setsid:     true,
			},
		}

		os.StartProcess(abspath,os.Args, attr)
		return
	}

	tmp,_:=ioutil.TempDir(os.Getenv("HOME"),"mount-"+os.Args[2])

	cf:=exec.Command("file","-b",imgpath)
	result,_:=cf.StdoutPipe()
	cf.Start()
	out,_:=ioutil.ReadAll(result)
	cf.Wait()

	if strings.Contains(string(out),"filesystem") {
		if e:=runCmd("mount",imgpath,tmp).do();e!=nil{
			fmt.Println("mount error.")
			os.RemoveAll(tmp)
			return

		}
	} else if strings.Contains(string(out),"compressed") {
		fmt.Println("mounting... pls wait.")
		if e:=runCmd("archivemount","-o","nonempty",imgpath,tmp).do();e!=nil{
			fmt.Println("archivemount error.")
			os.RemoveAll(tmp)
			return


		}
	} else {
		fmt.Println("mount file error.")
		os.RemoveAll(tmp)
		return
	}


	fmt.Println(tmp)

	for {
		time.Sleep(3*time.Second)
		cmd:=exec.Command("mount","-l")
		stdout,_:=cmd.StdoutPipe()
		cmd.Start()
		out:=bufio.NewScanner(stdout)
		tmpmount:=false
		for out.Scan() {
			if strings.Contains(out.Text(),tmp) {
				tmpmount=true
			}
		}
		cmd.Wait()
		if !tmpmount {
			os.RemoveAll(tmp)
			break
		}
	}

}

func pullto(name,templ,tag string)  {

	r:=rand.New(rand.NewSource(time.Now().UnixNano())).Int63()
	templpath:=filepath.Join(imgroot,templ)
	name += "-" +strconv.Itoa(int(r))
	imgpath:= imgroot + name


	e:=runCmd("qemu-img","create", "-f", "qcow2", "-o", "preallocation=metadata", imgpath, "2G").do()
	if e!=nil{
		fmt.Println(e)
		os.RemoveAll(imgpath)
		return }
	runCmd("mkfs.ext4","-F",imgpath).do()
	tmp,err:=ioutil.TempDir("/tmp","kyum")
	if err != nil {return}
	runCmd("mount",imgpath,tmp).do()
	os.Chdir(tmp)
	if _,e:=os.Stat(templpath);e!=nil{
		fmt.Println("template not exist.")
		os.Chdir(os.Getenv("HOME"))
		runCmd("umont",tmp).do()
		os.RemoveAll(tmp)
		os.RemoveAll(imgpath)
		return

	}
	if e:=runCmd("tar","-xf",templpath).do();e!=nil{
		fmt.Println("unpack not error.",e)
		os.Chdir(os.Getenv("HOME"))
		runCmd("umont",tmp).do()
		os.RemoveAll(tmp)
		os.RemoveAll(imgpath)
		return
	}
	fmt.Println("yum command:")
	scaninput:=bufio.NewScanner(os.Stdin)
	for scaninput.Scan() {
		cmd:=scaninput.Text()
		if len(cmd) ==0 {
			fmt.Print("yum command:")
			continue
		}
		if cmd == "exit"{break}
		as:=strings.Fields(cmd)
		args:=[]string{"-y","--nogpgcheck","--installroot="+tmp}
		for _,s:=range as{
			args=append(args,s)
		}
		runCmd("yum",args...).stdio().do()
		fmt.Print("yum command:")

	}

	reg:=regexp.MustCompile(`initramfs-.*x86_64.img`)
	var  initramfs string
	filepath.Walk(tmp, func(path string, info os.FileInfo, err error) error {
		if s:=reg.FindString(path);len(s) != 0{
			initramfs=s
		}
		return nil
	})

	if len(initramfs) == 0 {
		fmt.Println("boot kernel error. pls re pull")
		os.Chdir(os.Getenv("HOME"))
		runCmd("umont",tmp).do()
		os.RemoveAll(tmp)
		os.RemoveAll(imgpath)
		return
	}

	vmlinuz:=strings.Replace(initramfs,"initramfs","vmlinuz",1)
	vmlinuz=string([]rune(vmlinuz)[:len(vmlinuz)-4])

	initramfspath:=filepath.Join("/boot",initramfs)
	vmlinuzpath:=filepath.Join("/boot",vmlinuz)

	if _,e:=os.Stat(initramfspath);e!=nil{

		runCmd("cp",filepath.Join(tmp,"boot",initramfs),"/boot").do()
		runCmd("cp",filepath.Join(tmp,"boot",vmlinuz),"/boot").do()

	}

	os.Chdir(os.Getenv("HOME"))
	runCmd("umount",tmp).do()
	os.RemoveAll(tmp)


	runCmd("virt-install","--name" ,name , "--ram", "512", "--disk", imgpath ,"--noautoconsole","--boot",
			"kernel="+vmlinuzpath+",initrd="+initramfspath+",kernel_args=console=ttyS0 root=/dev/sda","--serial=pty","--noautoconsole").stdio().do()
	runCmd("virsh","list","--all").stdio().do()

}



func main()  {



	if len(os.Args) <2 {
		return
	}

	switch os.Args[1] {

	case "pull":
		if len(os.Args) <3 {return}
		v:=strings.Split(os.Args[2],"centos")
		if len(v) != 2 {return }
		pull(os.Args[2],v[1])

	case "pullto":
		if len(os.Args) <3 {return}
		v:=strings.Split(os.Args[2],":")
		if len(v) != 2 {return }

		pullto(os.Args[2],v[0],v[1])

	case "mt":
		if len(os.Args) <3 {return}
		if _,e:=os.Stat(filepath.Join(imgroot,os.Args[2]));e!=nil{
			fmt.Println(e)
			return
		}
		imgpath:=filepath.Join(imgroot,os.Args[2])
		monitm(imgpath)

	case "ins":

		if len(os.Args) <3 {return}
		imgpath:=filepath.Join(imgroot,os.Args[2])
		runCmd("virsh","destroy",os.Args[2])
		tmp,_:=ioutil.TempDir(os.Getenv("HOME"),"mount-"+os.Args[2])

		cf:=exec.Command("file","-b",imgpath)
		result,_:=cf.StdoutPipe()
		cf.Start()
		out,_:=ioutil.ReadAll(result)
		cf.Wait()

		if strings.Contains(string(out),"filesystem") {
			runCmd("mount",imgpath,tmp).do()

		} else {
			fmt.Println("mount format error.")
			os.RemoveAll(tmp)
			return
		}

		fmt.Println("yum command:")
		runCmd("chroot","")
		scaninput:=bufio.NewScanner(os.Stdin)
		for scaninput.Scan() {
			cmd:=scaninput.Text()
			if len(cmd) ==0 {
				fmt.Print("yum command:")
				continue
			}
			if cmd == "chroot" {
				if e:=runCmd("chroot",tmp).stdio().do();e!=nil{
					fmt.Println("os error. pls pullto install")
				}
			}
			if cmd == "exit"{break}
			as:=strings.Fields(cmd)
			args:=[]string{"-y","--nogpgcheck","--installroot="+tmp}
			for _,s:=range as{
				args=append(args,s)
			}
			runCmd("yum",args...).stdio().do()
			fmt.Print("yum command:")
		}
		runCmd("umount",tmp).do()
		os.RemoveAll(tmp)
	case "ls":
		runCmd("virsh","list","--all").stdio().do()
	case "st":
		if len(os.Args) <3 {return}
		runCmd("virsh","start",os.Args[2],"--console").stdio().do()
	case "co":
		if len(os.Args) <3 {return}
		runCmd("virsh","console",os.Args[2]).stdio().do()

	case "of":
		if len(os.Args) <3 {return}
		runCmd("virsh","destroy",os.Args[2]).stdio().do()
	case "dl":
		if len(os.Args) <3 {return}
		runCmd("virsh","undefine","--domain",os.Args[2]).stdio().do()
		os.RemoveAll(filepath.Join(imgroot,os.Args[2]))

	default:
		fmt.Println(`
version: v0.1;el7-x86_64)
Usage:
	pull centos[ver.] 		exmple: kyum pull centos6.9
	pull centos[samplename]:tag	exmple: kyum pullto centos6.9:mysql
	mt   mount vm 			exmple: kyum mt centos6.9:mysql
	ins  add package		exmple: kyum ins centos6.9:mysql [yum option]
	ls   list vm			exmple: kyum ls
	st   start vm			exmple: kyum st centos6.9:mysql  ( ctrl + ] exit )
	co   console vm			exmple: kyum console centos6.9:mysql
	of   stop vm			exmple: kyum stop centos6.9:mysql
	dl   delete vm			exmple: kyum dl centos6.9:mysql
		`)
		return
	}


}
