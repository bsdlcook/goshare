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

func init() {
	rand.Seed(time.Now().UnixNano())
}

func Check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func GetConfig() XShareConfig {
	ConfigFile, err := ioutil.ReadFile(fmt.Sprintf("/home/%s/.config/xshare/settings.json", os.Getenv("USER")))
	Check(err)

	Config := XShareConfig{}
	json.Unmarshal(ConfigFile, &Config)

	return Config
}

func PrintConfig() {
	Config := GetConfig()
	fmt.Printf("*** XShare Settings ***\n(Loaded from /home/%s/.config/xshare/settings.json)\n\nHost: \t\t\t%s:%s\nRemote Directory: \t%s\nRemote Url: \t\t%s\nFile Length: \t\t%d\nShow Extension: \t%t\n",
		os.Getenv("USER"),
		Config.Host,
		Config.Port,
		Config.RemoteDir,
		Config.RemoteUrl,
		Config.FileLen,
		Config.ShowExt)
}

func ReadPublicKey() (ssh.AuthMethod, error) {
	Buffer, err := ioutil.ReadFile(fmt.Sprintf("/home/%s/.ssh/id_rsa", os.Getenv("USER")))
	Check(err)

	Key, err := ssh.ParsePrivateKey(Buffer)
	Check(err)

	return ssh.PublicKeys(Key), err
}

func ConnectServer(Config XShareConfig) (*ssh.Client, error) {
	PublicKey, err := ReadPublicKey()
	Check(err)

	ClientConfig := &ssh.ClientConfig{
		User: os.Getenv("USER"),
		Auth: []ssh.AuthMethod{
			PublicKey,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	Addr      := fmt.Sprintf("%s:%s", Config.Host, Config.Port)
	Conn, err := ssh.Dial("tcp", Addr, ClientConfig)
	Check(err)

	return Conn, nil
}

func ReadLocalFile(File *string) io.Reader {
	if !filepath.IsAbs(*File) {
		*File, _ = filepath.Abs(*File)
	}

	Data, err := ioutil.ReadFile(*File)
	Check(err)

	return bytes.NewReader(Data)
}

func Screenshot() io.Reader {
	Output, err := exec.Command("maim", "-s").Output()
	Check(err)

	return bytes.NewReader(Output)
}

func GenRandomChars(length uint8) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	chars   := make([]rune, length)
	
	for i := range chars {
		chars[i] = letters[rand.Intn(len(letters))]
	}

	return string(chars)
}

func UploadFile(Config XShareConfig, Flags XShareFlags) (string) {
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
		return string(fmt.Sprintf("%s%s", Config.RemoteUrl, FileName))
	}

	return string(fmt.Sprintf("%s%s", Config.RemoteUrl, FileName[:len(FileName)-len(filepath.Ext(FileName))]))
}

func main() {
	Screenshot := flag.Bool("c", false, "Capture a screenshot with maim.")
	KeepName   := flag.Bool("k", false, "Keep the original name of the local file to upload oppose to random.")
	Clipboard  := flag.Bool("p", false, "Copy the file URL to clipboard when the file has uploaded.")
	Settings   := flag.Bool("s", false, "Show the current configuration settings.")
	LocalFile  := flag.String("f", "", "Select local file to upload.")

	flag.Parse()
	if *Settings {
		PrintConfig()
		os.Exit(1)
	} else if !*Screenshot && *LocalFile == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	Flags   := XShareFlags{*Screenshot, *LocalFile, *KeepName}
	FileUrl	:= UploadFile(GetConfig(), Flags)

	if *Clipboard {
		clipboard.WriteAll(FileUrl)
	} else {
		fmt.Printf("%s\n", FileUrl)
	}
}
