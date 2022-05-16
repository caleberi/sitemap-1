package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	parser "github.com/caleberi/linkparser/lib"
	home "github.com/mitchellh/go-homedir"
)

var filePath string
var maxDepth int

func init() {
	flag.StringVar(&filePath, "file-path", "link.txt", "provide file path to use to build map")
	flag.IntVar(&maxDepth, "max-depth", 10, "the maximum number of links deep to traverse")
	flag.Parse()
}

type loc struct {
	Value string `xml:"loc"`
}

type urlset struct {
	Urls  []loc  `xml:"url"`
	Xmlns string `xml:"xmlns,attr"`
}

func check(err error) {
	if err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}
}

func parseSiteFromFilePath(filePath string) ([]string, error) {
	var sites []string

	dir, err := home.Dir()
	if err != nil {
		return nil, errors.New("error while extracting home dir")
	}
	path := fmt.Sprintf("%s/%s", dir, filePath)
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read file : [%s]", path)
	}

	content := string(bs)
	// clean file contents
	content = strings.ReplaceAll(content, "\r", "")

	sites = append(sites, strings.Split(content, "\n")...)

	return sites, err
}

func retieveHtmlContent(url string) ([]byte, error) {
	client := &http.Client{}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error while reading content response of  link [%s] to bytes", url)
	}
	return bs, err
}

func withPrefix(prefix string) func(string) bool {
	return func(url string) bool {
		return strings.HasPrefix(url, prefix)
	}
}

func createSiteMap(site string) ([]string, error) {
	var ret []string

	ctnt, err := retieveHtmlContent(site)
	if err != nil {
		return nil, err
	}

	links, err := parser.Parse(bytes.NewReader(ctnt))

	if err != nil {
		return nil, err
	}

	uri, err := url.Parse(site)

	if err != nil {
		return nil, err
	}

	base := uri.Scheme + "://" + uri.Host

	for _, link := range links {
		switch {
		case strings.HasPrefix(link.Href, "/"):
			ret = append(ret, base+link.Href)
		case strings.HasPrefix(link.Href, "http"):
			ret = append(ret, link.Href)
		}
	}
	return filter(ret, withPrefix(base)), nil
}

func filter(links []string, fn func(string) bool) []string {
	ret := []string{}
	for _, link := range links {
		if fn(link) {
			ret = append(ret, link)
		}
	}
	return ret
}

func bfs(url string, maxdepth int) ([]string, error) {
	seen := make(map[string]struct{})
	var queue map[string]struct{}
	nq := map[string]struct{}{
		url: {},
	}

	for i := 0; i <= maxdepth; i++ {
		queue, nq = nq, make(map[string]struct{})
		if len(queue) == 0 {
			break
		}
		for url := range queue {
			if _, ok := seen[url]; ok {
				continue
			}
			seen[url] = struct{}{}
			links, err := createSiteMap(url)
			if err != nil {
				return nil, err
			}
			for _, link := range links {
				nq[link] = struct{}{}
			}
		}
	}

	ret := make([]string, 0, len(seen))
	for url := range seen {
		ret = append(ret, url)
	}
	return ret, nil

}

func main() {

	sites, err := parseSiteFromFilePath(filePath)

	if err != nil {
		log.Fatalln(err)
	}

	cwd, err := os.Getwd()

	if err != nil {
		log.Fatalln(err)
	}

	for _, site := range sites {
		pages, err := bfs(site, maxDepth)

		check(err)

		var toXml urlset
		for _, page := range pages {
			toXml.Urls = append(toXml.Urls, loc{page})
		}

		uri, err := url.Parse(site)

		if err != nil {
			log.Fatalln(err)
		}

		outdir := path.Join(cwd, uri.Host)
		if _, err := os.Stat(outdir); os.IsNotExist(err) {
			err := os.MkdirAll(outdir, os.ModePerm)
			if err != nil {
				log.Fatalln(err)
			}
		}

		fs, err := os.Create(filepath.Join(outdir, uri.Host+".xml"))
		fs.Write([]byte(xml.Header))

		if err != nil {
			log.Fatalln(err)
		}

		defer fs.Close()

		enc := xml.NewEncoder(fs)

		enc.Indent("", "  ")
		if err := enc.Encode(toXml); err != nil {
			panic(err)
		}
		fs.Write([]byte("\n</xml>"))
		fmt.Println("Done ...📦")
	}
}
