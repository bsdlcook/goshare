package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/atotto/clipboard"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type XShareFlags struct {
	Sreenshot bool
	LocalFile string
	KeepName  bool
}

type XShareConfig struct {
	Host      string `json:"host"`
	Port      string `json:"port"`
	RemoteDir string `json:"remoteDir"`
	RemoteUrl string `json:"remoteUrl"`
	FileLen   uint8  `json:"fileLen"`
	ShowExt   bool   `json:"showExtUrl"`
}

func GetConfig() XShareConfig {
	ConfigFile, err := ioutil.ReadFile(fmt.Sprintf("/home/%s/.config/xshare/settings.json", os.Getenv("USER")))
	if err != nil {
		log.Fatal(err)
	}

	Config := XShareConfig{}
	json.Unmarshal(ConfigFile, &Config)

	return Config
}

func ReadPublicKey() (ssh.AuthMethod, error) {
	Buffer, err := ioutil.ReadFile(fmt.Sprintf("/home/%s/.ssh/id_rsa", os.Getenv("USER")))
	if err != nil {
		return nil, err
	}

	Key, err := ssh.ParsePrivateKey(Buffer)
	if err != nil {
		return nil, err
	}

	return ssh.PublicKeys(Key), err
}

func ConnectServer(Config XShareConfig) (*ssh.Client, error) {
	PublicKey, err := ReadPublicKey()
	if err != nil {
		return nil, err
	}

	ClientConfig := &ssh.ClientConfig{
		User: os.Getenv("USER"),
		Auth: []ssh.AuthMethod{
			PublicKey,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	Addr := fmt.Sprintf("%s:%s", Config.Host, Config.Port)
	Conn, err := ssh.Dial("tcp", Addr, ClientConfig)
	if err != nil {
		return nil, err
	}

	return Conn, nil
}

func ReadLocalFile(File *string) io.Reader {
	if !filepath.IsAbs(*File) {
		*File, _ = filepath.Abs(*File)
	}

	Data, err := ioutil.ReadFile(*File)
	if err != nil {
		log.Fatal(err)
	}

	return bytes.NewReader(Data)
}

func Screenshot() io.Reader {
	Output, err := exec.Command("maim", "-s").Output()
	if err != nil {
		return nil
	}

	return bytes.NewReader(Output)
}

func GenRandomChars(length uint8) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	chars := make([]rune, length)
	for i := range chars {
		chars[i] = letters[rand.Intn(len(letters))]
	}

	return string(chars)
}

func Check(err error) (string, error) {
	if err != nil {
		return "", err
	}

	return "", nil
}

func UploadFile(Config XShareConfig, Flags XShareFlags) (string, error) {
	Conn, err := ConnectServer(Config)
	Check(err)

	Client, err := sftp.NewClient(Conn)
	Check(err)

	var FileName string
	switch {
		case Flags.KeepName && len(Flags.LocalFile) > 0:
			FileName = Flags.LocalFile
		case len(Flags.LocalFile) > 0:
			FileName = GenRandomChars(Config.FileLen) + filepath.Ext(Flags.LocalFile)
		default:
			FileName = GenRandomChars(Config.FileLen) + ".png"
	}

	File, err := Client.Create(fmt.Sprintf("%s%s", Config.RemoteDir, FileName))
	Check(err)

	var Output io.Reader
	if len(Flags.LocalFile) > 0 {
		Output = ReadLocalFile(&Flags.LocalFile)
	} else {
		Output = Screenshot()
	}

	Data, err := ioutil.ReadAll(Output)
	Check(err)

	_, err = File.Write(Data)
	Check(err)

	defer func() {
		Conn.Close()
		Client.Close()
		File.Close()
	}()

	if Config.ShowExt {
		return string(fmt.Sprintf("%s%s", Config.RemoteUrl, FileName)), nil
	}

	return string(fmt.Sprintf("%s%s", Config.RemoteUrl, FileName[:len(FileName)-len(filepath.Ext(FileName))])), nil
}

func init() { rand.Seed(time.Now().UnixNano()) }
func main() {
	Screenshot := flag.Bool("s", false, "Take a screenshot with maim")
	LocalFile := flag.String("f", "", "Select local file to upload")
	KeepName := flag.Bool("k", false, "Keep the original name of the local file to upload oppose to random")
	Clipboard := flag.Bool("c", false, "Copy the file URL to clipboard when the file has uploaded")
	flag.Parse()

	if !*Screenshot && *LocalFile == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	Flags := XShareFlags{*Screenshot, *LocalFile, *KeepName}
	FileUrl, err := UploadFile(GetConfig(), Flags)
	if err != nil {
		log.Fatal(err)
	}

	if *Clipboard {
		clipboard.WriteAll(FileUrl)
	} else {
		fmt.Print(FileUrl)
	}
}
