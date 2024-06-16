package main

import (
	"errors"
	"fmt"
	"github.com/goharbor/harbor/src/lib/log"
	sftppkg "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func main() {

	conf := &ssh.ClientConfig{}
	conf.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	conf.User = "ftp_user_1"
	conf.Auth = append(conf.Auth, ssh.Password("ftp_user_1"))

	hostname := fmt.Sprintf("%s:%s", "localhost", "2022")

	conn, err := ssh.Dial("tcp", hostname, conf)
	if err != nil {
		log.Fatal("dial ", err)
	}

	client, err := sftppkg.NewClient(conn, sftppkg.MaxPacketUnchecked(32768*1024))
	if err != nil {
		log.Fatal("new client", err)
	}
	defer client.Close()

	files, err := client.ReadDir("/docker/registry/v2/repositories/asdf/library/library/gitlab")
	if err != nil {
		if errors.Is(err, sftppkg.ErrSSHFxConnectionLost) {
			log.Fatal("connection lost:", client.Wait())
		}
	}
	fmt.Println(len(files))
}
