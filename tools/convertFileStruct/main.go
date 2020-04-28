// Convert old file structure to new structure.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/anaminus/but"
)

type Build struct {
	Hash    string
	Date    time.Time
	Version string
}

type Metadata struct {
	GUID    string
	Date    time.Time
	Version string
}

func ReadBuilds(root string) (builds []Build) {
	root = filepath.Join(root, "builds.json")
	b, err := ioutil.ReadFile(root)
	but.IfFatal(err, "read builds.json")
	but.IfFatal(json.Unmarshal(b, &builds), "decode builds.json")
	return builds
}

func CopyFile(hash, name, dst, src string) {
	s, err := os.Open(src)
	but.IfFatalf(err, "%s:%s: open src", hash, name)
	d, err := os.Create(dst)
	but.IfFatalf(err, "%s:%s: create dst", hash, name)
	_, err = io.Copy(d, s)
	but.IfFatalf(err, "%s:%s: copy", hash, name)
	s.Close()
	d.Close()
}

func main() {
	root := "../.."
	src := filepath.Join(root, "data")
	dst := filepath.Join(root, "data2", "legacy")
	builds := ReadBuilds(src)
	var meta []Metadata
	for i, build := range builds {
		dir := filepath.Join(dst, "builds", build.Hash)
		but.IfFatal(os.MkdirAll(dir, 0755))
		meta = append(meta, Metadata{
			GUID:    build.Hash,
			Date:    build.Date,
			Version: build.Version,
		})
		CopyFile(build.Hash, "api-dump.json",
			filepath.Join(dir, "API-Dump.json"),
			filepath.Join(src, "api-dump", "json", build.Hash+".json"),
		)
		fmt.Printf("write %d/%d %s/%s\n", i+1, len(builds), build.Hash, "API-Dump.json")
		CopyFile(build.Hash, "api-dump.txt",
			filepath.Join(dir, "API-Dump.txt"),
			filepath.Join(src, "api-dump", "txt", build.Hash+".txt"),
		)
		fmt.Printf("write %d/%d %s/%s\n", i+1, len(builds), build.Hash, "API-Dump.txt")
		CopyFile(build.Hash, "reflection-metadata",
			filepath.Join(dir, "ReflectionMetadata.xml"),
			filepath.Join(src, "reflection-metadata", "xml", build.Hash+".xml"),
		)
		fmt.Printf("write %d/%d %s/%s\n", i+1, len(builds), build.Hash, "ReflectionMetadata.xml")
	}

	f, err := os.Create(filepath.Join(dst, "metadata.json"))
	but.IfFatal(err, "create metadata.json")
	e := json.NewEncoder(f)
	e.SetEscapeHTML(false)
	e.SetIndent("", "\t")
	but.IfFatal(e.Encode(&meta), "encode metadata.json")
	fmt.Printf("write metadata.json\n")
	f.Close()

	f, err = os.Create(filepath.Join(dst, "latest.json"))
	but.IfFatal(err, "create latest.json")
	e = json.NewEncoder(f)
	e.SetEscapeHTML(false)
	e.SetIndent("", "\t")
	but.IfFatal(e.Encode(&meta[len(meta)-1]), "encode latest.json")
	fmt.Printf("write latest.json\n")
	f.Close()
}
