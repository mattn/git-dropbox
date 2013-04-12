package main

import (
	"crypto/sha1"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/garyburd/go-oauth/oauth"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

var nocache = flag.Bool("nocache", false, "Don't store caches")

type resList struct {
	Contents []struct {
		Path string `json:"path"`
	} `json:"contents"`
}

type resStore struct {
	Path string `json:"path"`
}

func openBrowser(url_ string) {
	cmd := "xdg-open"
	args := []string{cmd, url_}
	if runtime.GOOS == "windows" {
		cmd = "rundll32.exe"
		args = []string{cmd, "url.dll,FileProtocolHandler", url_}
	} else if runtime.GOOS == "darwin" {
		cmd = "open"
		args = []string{cmd, url_}
	}
	cmd, err := exec.LookPath(cmd)
	if err != nil {
		log.Fatal("command not found:", err)
	}
	p, err := os.StartProcess(cmd, args, &os.ProcAttr{Dir: "", Files: []*os.File{nil, nil, os.Stderr}})
	if err != nil {
		log.Fatal("failed to start command:", err)
	}
	defer p.Release()
}

func getConfig() (string, map[string]string) {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err.Error())
	}
	user.Current()
	dir := filepath.Join(usr.HomeDir, ".config")
	if runtime.GOOS == "windows" {
		dir = os.Getenv("APPDATA")
	}
	_, err = os.Lstat(dir)
	if err != nil {
		if os.Mkdir(dir, 0700) != nil {
			log.Fatal("failed to create directory:", err)
		}
	}
	dir = filepath.Join(dir, "git-dropbox")
	_, err = os.Lstat(dir)
	if err != nil {
		if os.Mkdir(dir, 0700) != nil {
			log.Fatal("failed to create directory:", err)
		}
	}
	file := filepath.Join(dir, "settings.json")
	config := map[string]string{}

	b, err := ioutil.ReadFile(file)
	if err != nil {
		config["ClientToken"] = "9q3p2mkl6duw9tx"
		config["ClientSecret"] = "g70xsgrop7e6ac0"
	} else {
		err = json.Unmarshal(b, &config)
		if err != nil {
			log.Fatal("could not unmarhal settings.json:", err)
		}
	}
	return file, config
}

func getClient() (*oauth.Client, *oauth.Credentials, error) {
	file, config := getConfig()

	oauthClient := &oauth.Client{
		TemporaryCredentialRequestURI: "https://api.dropbox.com/1/oauth/request_token",
		ResourceOwnerAuthorizationURI: "https://www.dropbox.com/1/oauth/authorize",
		TokenRequestURI:               "https://api.dropbox.com/1/oauth/access_token",
	}
	oauthClient.Credentials = oauth.Credentials{
		Token:  config["ClientToken"],
		Secret: config["ClientSecret"],
	}

	var newCred *oauth.Credentials
	if _, ok := config["AccessToken"]; !ok {
		var err error
		var conn net.Listener
		var port int
		for port = 5000; port < 9000; port++ {
			conn, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, nil, err
		}

		tempCred, err := oauthClient.RequestTemporaryCredentials(http.DefaultClient, "", nil)
		if err != nil {
			return nil, nil, err
		}

		url_ := oauthClient.AuthorizationURL(tempCred, url.Values{
			"oauth_callback": {fmt.Sprintf("http://localhost:%d/", port)}})
		openBrowser(url_)

		err = nil
		http.Serve(conn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			oauth_token := r.URL.Query().Get("oauth_token")
			if oauth_token != "" {
				newCred, _, err = oauthClient.RequestToken(http.DefaultClient, tempCred, oauth_token)
			}
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`
<script>
	window.open("about:blank", "_self").close();
</script>
<body>
	Close this window
</body>
			`))
			defer conn.Close()
		}))

		if err != nil {
			return nil, nil, err
		}
		config["AccessToken"] = newCred.Token
		config["AccessSecret"] = newCred.Secret

		b, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			log.Fatal("failed to store file:", err)
		}
		err = ioutil.WriteFile(file, b, 0700)
		if err != nil {
			log.Fatal("failed to store file:", err)
		}
	} else {
		newCred = new(oauth.Credentials)
		newCred.Token = config["AccessToken"]
		newCred.Secret = config["AccessSecret"]
	}

	return oauthClient, newCred, nil
}

func assetDir() string {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return filepath.Join(usr.HomeDir, ".gitasset")
}

func cachePath(sha1hex string) (dirpath, filename string) {
	dirpath = filepath.Join(assetDir(), "data", string(sha1hex[0:2]), string(sha1hex[2:4]))
	filename = string(sha1hex[4:])

	fullpath := filepath.Join(dirpath, filename)
	if _, err := os.Lstat(fullpath); os.IsExist(err) {
		return
	}
	if err := os.MkdirAll(dirpath, os.ModePerm); err != nil {
		log.Fatal(err)
	}
	return
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go-dropbox [load|store|drop|list]")
	os.Exit(1)
}

func load(hex string) {
	var dirpath, filename string
	if *nocache == false {
		dirpath, filename = cachePath(hex)
		if stream, err := os.Open(filepath.Join(dirpath, filename)); err == nil {
			_, err = io.Copy(os.Stdout, stream)
			if err != nil {
				log.Fatal(err)
			}
			return
		}
	}

	client, cred, err := getClient()
	if err != nil {
		log.Fatal(err.Error())
	}

	url_ := "https://api-content.dropbox.com/1/files/sandbox/" + hex
	params := make(url.Values)
	client.SignParam(cred, "GET", url_, params)
	res, err := http.Get(url_ + "?" + params.Encode())
	if err != nil {
		log.Fatal(err.Error())
	}
	defer res.Body.Close()

	var writer io.Writer
	if *nocache == false {
		file, err := os.Create(filepath.Join(dirpath, filename))
		if err != nil {
			log.Fatal(err.Error())
		}
		writer = io.MultiWriter(os.Stdout, file)
	} else {
		writer = os.Stdout
	}
	io.Copy(writer, res.Body)
}

func store(hex string) {
	var dirpath, filename string
	var file io.Writer
	var err error

	if *nocache == false {
		dirpath, filename = cachePath(hex)
		file, err = os.Create(filepath.Join(dirpath, filename))
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	var size int64
	if *nocache == false {
		_, err = os.Stdin.Seek(0, os.SEEK_SET)
		if err != nil {
			log.Fatal(err.Error())
		}
		size, err = io.Copy(file, os.Stdin)
		if err != nil {
			log.Fatal(err.Error())
		}
	} else {
		size, err = os.Stdin.Seek(0, os.SEEK_END)
	}

	client, cred, err := getClient()
	if err != nil {
		log.Fatal(err.Error())
	}

	url_ := "https://api-content.dropbox.com/1/files_put/sandbox/" + hex
	params := make(url.Values)
	params.Add("overwrite", "true")
	client.SignParam(cred, "PUT", url_, params)

	_, err = os.Stdin.Seek(0, os.SEEK_SET)
	if err != nil {
		log.Fatal(err.Error())
	}

	req, err := http.NewRequest("PUT", url_+"?"+params.Encode(), os.Stdin)
	if err != nil {
		log.Fatal(err.Error())
	}
	req.ContentLength = size

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer res.Body.Close()
	var obj resStore
	json.NewDecoder(res.Body).Decode(&obj)
	fmt.Println(obj.Path)
}

func drop(hex string) {
	client, cred, err := getClient()
	if err != nil {
		log.Fatal(err.Error())
	}

	url_ := "https://api.dropbox.com/1/fileops/delete"
	params := make(url.Values)
	params.Add("root", "sandbox")
	params.Add("path", hex)
	client.SignParam(cred, "POST", url_, params)
	res, err := http.PostForm(url_, params)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer res.Body.Close()

	if *nocache == false {
		dirpath, filename := cachePath(hex)
		err = os.Remove(filepath.Join(dirpath, filename))
		if err != nil {
			log.Fatal(err.Error())
		}
	}
}

func list() {
	client, cred, err := getClient()
	if err != nil {
		log.Fatal(err.Error())
	}

	url_ := "https://api.dropbox.com/1/metadata/sandbox"
	params := make(url.Values)
	client.SignParam(cred, "POST", url_, params)
	res, err := http.PostForm(url_, params)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer res.Body.Close()
	var obj resList
	json.NewDecoder(res.Body).Decode(&obj)
	for _, content := range obj.Contents {
		fmt.Println(content.Path[1:])
	}
}

func main() {
	if len(os.Args) == 1 {
		usage()
	}

	temp, err := ioutil.TempFile("", "")
	if err != nil {
		log.Fatal(err.Error())
	}
	oldStdin := os.Stdin
	defer func() {
		os.Stdin = oldStdin
		os.Remove(temp.Name())
	}()
	io.Copy(temp, os.Stdin)
	os.Stdin = temp
	os.Stdin.Seek(0, os.SEEK_SET)

	hex := ""
	if os.Args[1] == "load" {
		if len(os.Args) == 2 {
			hash, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				log.Fatal(err.Error())
			}
			hex = strings.TrimSpace(string(hash))
		} else {
			hex = os.Args[2]
		}

		load(hex)
	} else if os.Args[1] == "store" {
		if len(os.Args) == 2 {
			sha1h := sha1.New()
			_, err := io.Copy(sha1h, os.Stdin)
			if err != nil {
				log.Fatal(err.Error())
			}
			hex = fmt.Sprintf("%x", sha1h.Sum([]byte{}))
		} else {
			hex = os.Args[2]
		}

		store(hex)
	} else if os.Args[1] == "drop" {
		if len(os.Args) == 2 {
			hash, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				log.Fatal(err)
			}
			hex = strings.TrimSpace(string(hash))
		} else {
			hex = os.Args[2]
		}

		drop(hex)
	} else if os.Args[1] == "list" {
		list()
	} else {
		usage()
	}
}
