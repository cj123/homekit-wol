package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/brutella/hc"
	"github.com/brutella/hc/accessory"
	"github.com/ghthor/gowol"
	"github.com/sparrc/go-ping"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
)

func main() {
	var hkConfig *HomekitWOL

	executable, err := os.Executable()
	checkError(err)

	b, err := ioutil.ReadFile(filepath.Join(filepath.Dir(executable), "config.yml"))
	checkError(err)

	err = yaml.Unmarshal(b, &hkConfig)
	checkError(err)

	hkConfig.Run()
}

func checkError(err error) {
	if err != nil {
		log.Panic(err)
	}
}

type HomekitWOL struct {
	IP      string `yaml:"ip"`
	Mac     string `yaml:"mac"`
	Keyfile string `yaml:"keyfile"`
	User    string `yaml:"user"`
	Port    string `yaml:"port"`
	Pin     string `yaml:"pin"`
}

func (h *HomekitWOL) Run() {
	info := accessory.Info{
		Name: "PC WakeOnLan",
	}

	acc := accessory.NewSwitch(info)
	config := hc.Config{Pin: h.Pin}
	t, err := hc.NewIPTransport(config, acc.Accessory)

	if err != nil {
		log.Panic(err)
	}

	acc.Switch.On.OnValueRemoteUpdate(func(on bool) {
		if on {
			err := h.doWOL()

			if err != nil {
				log.Printf("unable to do wol, err: %s", err)
			}
		} else {
			err := h.sshSuspend()
			if err != nil {
				log.Printf("unable to do shutdown, err: %s", err)
			}
		}
	})

	ticker := time.NewTicker(time.Second * 10)

	go func() {
		h.updateOnStatus(acc)

		for range ticker.C {
			h.updateOnStatus(acc)
		}
	}()

	hc.OnTermination(func() {
		<-t.Stop()
	})

	t.Start()
}

func (h *HomekitWOL) doWOL() error {
	mp, err := wol.NewMagicPacket(h.Mac)

	if err != nil {
		return err
	}

	return mp.Send("255.255.255.255")
}

func (h *HomekitWOL) sshSuspend() error {
	sshConfig := &ssh.ClientConfig{
		User: h.User,
		Auth: []ssh.AuthMethod{
			publicKeyFile(h.Keyfile),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", h.IP+":"+h.Port, sshConfig)

	if err != nil {
		return err
	}

	defer conn.Close()

	sess, err := conn.NewSession()

	if err != nil {
		return err
	}

	defer sess.Close()

	return sess.Run("sudo systemctl suspend")
}

func publicKeyFile(file string) ssh.AuthMethod {
	buf, err := ioutil.ReadFile(file)

	if err != nil {
		panic(err)
		return nil
	}

	key, err := ssh.ParsePrivateKey(buf)
	if err != nil {
		panic(err)
		return nil
	}

	return ssh.PublicKeys(key)
}

func (h *HomekitWOL) updateOnStatus(acc *accessory.Switch) {
	on, err := h.doPing()

	if err != nil {
		log.Printf("ping failed, err: %s", err)
	}

	acc.Switch.On.SetValue(on)
}

func (h *HomekitWOL) doPing() (bool, error) {
	p, err := ping.NewPinger(h.IP)

	if err != nil {
		return false, err
	}
	p.SetPrivileged(true)

	p.Count = 3
	p.Run()

	return p.Statistics().PacketLoss == 0, nil
}
