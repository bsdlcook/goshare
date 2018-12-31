package main

import (
	"encoding/json"
	"bytes"
	"fmt"
	"sync"
	"math/rand"
	"time"
	"log"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"github.com/jessevdk/go-flags"
	"github.com/atotto/clipboard"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type XShareOptions struct {
	Screenshot bool     `short:"s" long:"screenshot" description:"Capture a screenshot with maim."`
	Clipboard  bool     `short:"c" long:"copy" description:"Copy the uploaded screenshot url to clipboard."`
	KeepName   bool     `short:"k" long:"keepname" description:"Keep local filename intact when uploading instead of randomized."`
	Files      []string `short:"f" long:"file" description:"Local file(s) to upload."`
}

type XShareConfig struct {
	Host       string `json:"host"`
	Port       string `json:"port"`
	RemoteDir  string `json:"remoteDir"`
	RemoteUrl  string `json:"remoteUrl"`
	FileLen    uint8  `json:"fileLen"`
	ShowExtUrl bool   `json:"showExtUrl"`
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

func GenRandomChars(Length uint8) string {
	Letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	Chars   := make([]rune, Length)
	
	for i := range Chars {
		Chars[i] = Letters[rand.Intn(len(Letters))]
	}

	return string(Chars)
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

var WaitGroup = sync.WaitGroup{}

func UploadFile(LocalFile string, Options XShareOptions) {
	Config := GetConfig()
	
	Conn, err := ConnectServer(Config)
	Check(err)

	Client, err := sftp.NewClient(Conn)
	Check(err)
	
	var FileName string
	switch {
		case Options.KeepName && len(Options.Files) >= 1:
			FileName = LocalFile
		case len(Options.Files) >= 1:
			FileName = GenRandomChars(Config.FileLen) + filepath.Ext(LocalFile)
		default:
			FileName = GenRandomChars(Config.FileLen) + ".png"
	}

	File, err := Client.Create(fmt.Sprintf("%s%s", Config.RemoteDir, FileName))
	Check(err)

	var Output io.Reader
	if len(Options.Files) >= 1 {
		Output = ReadLocalFile(&LocalFile)	
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
		
		var ScreenshotUrl string
		if Config.ShowExtUrl {
			ScreenshotUrl = fmt.Sprintf("%s%s", Config.RemoteUrl, FileName)
		} else {
			ScreenshotUrl = fmt.Sprintf("%s%s", Config.RemoteUrl, FileName[:len(FileName)-len(filepath.Ext(FileName))])
		}

		switch {
			case len(Options.Files) >= 1:
				fmt.Printf("%s: %s%s\n", LocalFile, Config.RemoteUrl, FileName)
				WaitGroup.Done()
			case Options.Clipboard:
				clipboard.WriteAll(fmt.Sprintf("%s", ScreenshotUrl))
			default:
				fmt.Printf("%s\n", ScreenshotUrl)
		} 
	}()
}

func ParseOptions(Options XShareOptions) {
	if len(Options.Files) >= 1 {
		WaitGroup.Add(len(Options.Files))
		for _, File := range Options.Files {
			go UploadFile(File, Options)
		}
		WaitGroup.Wait()
	} else {
		UploadFile("", Options)
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {	
	Opts   := XShareOptions{}
	Flags  := flags.NewParser(&Opts, flags.Default&^flags.HelpFlag)
	_, err := Flags.Parse()
	Check(err)

	if !Opts.Screenshot && len(Opts.Files) < 1 {
		Flags.WriteHelp(os.Stdout)
		os.Exit(1)
	}

	ParseOptions(Opts)
}
