package main

import (
	"net/http"

	"os"

	"regexp"
	"io/ioutil"
	"os/exec"

	"fmt"

	"strings"
	"math/rand"
	"time"
	"strconv"
	"path/filepath"
	"bufio"
)

// kyum pull centos7.9:mysql
// kyum

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




func runCmd(std bool,c string,args ...string) error {

	cmd:=exec.Command(c,args...)

	if std {
		cmd.Stdin=os.Stdin
		cmd.Stdout=os.Stdout
		cmd.Stderr=os.Stderr
	}

	if e:=cmd.Run();e !=nil {

		return e
	}
	return nil

}

func pull(name,ver,tag string) {


	r:=rand.New(rand.NewSource(time.Now().UnixNano())).Int63()
	name += "-" +strconv.Itoa(int(r))
	fmt.Printf("\r%s","1/6")
	imgpath:= imgroot + name
	if e:=runCmd(false,"qemu-img","create", "-f", "qcow2", "-o", "preallocation=metadata", imgpath, "1G");e != nil {
		fmt.Println(e)
		return
	}
	fmt.Printf("\r%s","2/6")
	runCmd(false,"mkfs.ext4","-F",imgpath)
	tmp,err:=ioutil.TempDir("/tmp","kyum")
	if err != nil {return}
	runCmd(false,"mount",imgpath,tmp)
	fmt.Printf("\r%s","3/6")
	if e:=runCmd(false,"rpm","-ivh","--root="+tmp,findcrpm(ver));e!=nil{
		runCmd(false,"umount",tmp)
		os.RemoveAll(tmp)
		os.RemoveAll(imgpath)
	}

	fmt.Printf("\r%s","4/6")

	fi,err:=os.Create(filepath.Join(tmp,"etc","fstab"))
	defer fi.Close()
	wr:=bufio.NewWriter(fi)
	wr.WriteString(`
LABEL=/                 /                       ext4    defaults        0 0
tmpfs                   /dev/shm                tmpfs   defaults        0 0
devpts                  /dev/pts                devpts  gid=5,mode=620  0 0
sysfs                   /sys                    sysfs   defaults        0 0
proc                    /proc                   proc    defaults        0 0`)
	wr.Flush()
	fi.Close()

	fmt.Printf("\r%s","5/6")
	if e:=runCmd(false,"yum","install","-y","--nogpgcheck","--installroot="+tmp,tag,"kernel");e!=nil{
		runCmd(false,"umount",tmp)
		os.RemoveAll(tmp)
		os.RemoveAll(imgpath)
	}
	reg:=regexp.MustCompile(`initramfs-.*x86_64.img`)
	var  initramfs string
	filepath.Walk(tmp, func(path string, info os.FileInfo, err error) error {
		if s:=reg.FindString(path);len(s) != 0{
			initramfs=s
		}
		return nil
	})

	if len(initramfs) == 0 {return}

	vmlinuz:=strings.Replace(initramfs,"initramfs","vmlinuz",1)
	vmlinuz=string([]rune(vmlinuz)[:len(vmlinuz)-4])

	initramfspath:=filepath.Join("/boot",initramfs)
	vmlinuzpath:=filepath.Join("/boot",vmlinuz)

	if _,e:=os.Stat(initramfspath);e!=nil{

		runCmd(false,"cp",filepath.Join(tmp,"boot",initramfs),"/boot")
		runCmd(false,"cp",filepath.Join(tmp,"boot",vmlinuz),"/boot")

	}

	runCmd(false,"umount",tmp)
	os.RemoveAll(tmp)
	fmt.Printf("\r%s","6/6")
	runCmd(false,"virt-install","--name" ,name , "--ram", "512", "--disk", imgpath ,"--boot",
		   "kernel="+vmlinuzpath+",initrd="+initramfspath+",kernel_args=console=ttyS0 root=/dev/sda","--serial=pty","--noautoconsole")

}


func ins(name,pak string) {

	imgpath:= imgroot + name
	runCmd(false,"virsh","destroy",name)
	tmp,err:=ioutil.TempDir("/tmp","kyum")
	if err != nil {return }
	runCmd(false,"mount",imgpath,tmp)
	if e:=runCmd(false,"yum","install","-y","--nogpgcheck","--installroot="+tmp,pak);e!=nil{
		fmt.Println(e)
		runCmd(false,"umount",tmp)
		os.RemoveAll(tmp)
	}
	runCmd(false,"umount",tmp)
	os.RemoveAll(tmp)

}


func chr(name string) {

	imgpath:= imgroot + name
	runCmd(false,"virsh","destroy",name)
	tmp,err:=ioutil.TempDir("/tmp","kyum")
	if err != nil {return }
	runCmd(false,"mount",imgpath,tmp)


	if e:=runCmd(true,"chroot",tmp);e!=nil{
		fmt.Println(e)
		runCmd(false,"umount",tmp)
		os.RemoveAll(tmp)
	}
	runCmd(false,"umount",tmp)
	os.RemoveAll(tmp)

}

func cmd(name ,cmd ,path string,)  {

	imgpath:= imgroot + name
	runCmd(false,"virsh","destroy",name)
	tmp,err:=ioutil.TempDir("/tmp","kyum")
	if err != nil {return }
	runCmd(false,"mount",imgpath,tmp)
	if e:=runCmd(true,cmd,filepath.Join(tmp,path));e!=nil{
		fmt.Println(e)
		runCmd(false,"umount",tmp)
		os.RemoveAll(tmp)

	}
	runCmd(false,"umount",tmp)
	os.RemoveAll(tmp)



}

func main(){



	if len(os.Args) <2 {
		return
	}


	switch os.Args[1] {

	case "pull":
		n:=strings.Split(os.Args[2],":")
		if len(n) != 2 {return }
		name,tag:=n[0],n[1]
		v:=strings.Split(name,"centos")
		if len(v) != 2 {return }
		ver:=v[1]
		pull(os.Args[2],ver,tag)
	case "ins":
		if len(os.Args) <3 {
			return
		}

		//centos6.9:mysql-5365705025944420096
		ins(os.Args[2],os.Args[3])
	case "chr":

		if len(os.Args) <2 {
			return
		}
		chr(os.Args[2])
	case "cmd":

		if len(os.Args) <3 {
			return
		}
		cmd(os.Args[2],os.Args[3],os.Args[4])

		
	default:
		return

	}

}