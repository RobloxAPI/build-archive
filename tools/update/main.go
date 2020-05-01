// Update production group.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/anaminus/but"
	"github.com/robloxapi/rbxfetch"
)

var ExpectedFiles = []expectedFile{
	{Name: "API-Dump.json", Location: "APIDump"},
	{Name: "ReflectionMetadata.xml", Location: "ReflectionMetadata"},
}

const RootPath = "../../data"
const GroupName = "production"
const BuildsDirName = "builds"
const MetadataFileName = "metadata.json"
const LatestFileName = "latest.json"

type expectedFile struct {
	Name     string
	Location string
}

type Build = rbxfetch.Build

type Metadata struct {
	Files   []string
	Builds  []Build
	Missing map[string][]string
}

func GetCurrentMetadata(root string) (meta Metadata) {
	f, err := os.Open(filepath.Join(root, MetadataFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return meta
		}
		but.IfFatal(err, "open metadata")
		return meta
	}
	defer f.Close()

	but.IfFatal(json.NewDecoder(f).Decode(&meta), "decode metadata")
	return meta
}

type BuildKey struct {
	GUID    string
	Date    int64
	Version rbxfetch.Version
}

type KnownBuilds map[BuildKey]bool

func (k KnownBuilds) Has(b Build) bool {
	return k[bkey(b)]
}

func (k KnownBuilds) Add(b Build) {
	k[bkey(b)] = true
}

func bkey(b Build) BuildKey {
	return BuildKey{
		GUID:    b.GUID,
		Date:    b.Date.Unix(),
		Version: b.Version,
	}
}

func FetchFile(client *rbxfetch.Client, guid, dstpath string, locs []rbxfetch.Location) error {
	var src io.ReadCloser
	for i, loc := range locs {
		_, rc, err := client.Get(loc, guid)
		if err != nil {
			if i < len(locs)-1 {
				continue
			}
			return fmt.Errorf("fetch file: %w", err)
		}
		src = rc
		break
	}
	defer src.Close()
	dst, err := os.Create(dstpath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer dst.Close()
	if _, err = io.Copy(dst, src); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func UpdateMetadata(root string, meta Metadata) {
	metadata, err := os.Create(filepath.Join(root, MetadataFileName))
	but.IfFatal(err, "update metadata")
	je := json.NewEncoder(metadata)
	je.SetEscapeHTML(false)
	je.SetIndent("", "\t")
	err = je.Encode(meta)
	metadata.Close()
	but.IfFatal(err, "encode metadata")
}

func FindLocation(name string) string {
	for _, file := range ExpectedFiles {
		if file.Name == name {
			return file.Location
		}
	}
	return ""
}

func main() {
	var retry bool
	flag.BoolVar(&retry, "retry", false, "Attempt to download known missing files.")
	flag.Parse()

	rootPath := filepath.Join(RootPath, GroupName)
	buildsPath := filepath.Join(rootPath, BuildsDirName)
	but.IfFatal(os.MkdirAll(buildsPath, 0755), "make builds directory")

	meta := GetCurrentMetadata(rootPath)
	meta.Files = make([]string, len(ExpectedFiles))
	for i, file := range ExpectedFiles {
		meta.Files[i] = file.Name
	}
	sort.Strings(meta.Files)
	if meta.Missing == nil {
		meta.Missing = map[string][]string{}
	}

	knownBuilds := KnownBuilds{}
	for _, build := range meta.Builds {
		knownBuilds.Add(build)
	}

	client := rbxfetch.NewClient()
	client.CacheMode = rbxfetch.CacheNone

	// Find new builds.
	builds, err := client.Builds()
	but.IfFatal(err, "fetch builds")

	// Fetch files.
	for _, build := range builds {
		if knownBuilds.Has(build) {
			continue
		}
		path := filepath.Join(buildsPath, build.GUID)
		if err := os.Mkdir(path, 0755); err != nil && !os.IsExist(err) {
			but.IfFatal(err, "make build directory")
		}
		var missing []string
		for _, file := range meta.Files {
			err := FetchFile(client,
				build.GUID,
				filepath.Join(buildsPath, build.GUID, file),
				client.Config.Locations[FindLocation(file)],
			)
			if err != nil {
				missing = append(missing, file)
				but.Log(err)
				continue
			}
			but.Logf("found file %s/%s\n", build.GUID, file)
		}
		if len(missing) > 0 {
			meta.Missing[build.GUID] = missing
		}
		meta.Builds = append(meta.Builds, build)
		UpdateMetadata(rootPath, meta)
	}

	sort.Slice(meta.Builds, func(i, j int) bool {
		return meta.Builds[i].Date.Before(meta.Builds[j].Date)
	})
	UpdateMetadata(rootPath, meta)

	// Write latest.
	latest, err := os.Create(filepath.Join(rootPath, LatestFileName))
	but.IfFatal(err, "update latest")
	if len(meta.Builds) == 0 {
		latest.Close()
		return
	}
	je := json.NewEncoder(latest)
	je.SetEscapeHTML(false)
	je.SetIndent("", "\t")
	err = je.Encode(meta.Builds[len(meta.Builds)-1])
	latest.Close()
	but.IfFatal(err, "encode latest")

	// Retry missing files.
	if !retry {
		return
	}
	for guid, files := range meta.Missing {
		var missing []string
		for _, file := range files {
			err := FetchFile(client,
				guid,
				filepath.Join(buildsPath, guid, file),
				client.Config.Locations[FindLocation(file)],
			)
			if err != nil {
				missing = append(missing, file)
				but.Log(err)
				continue
			}
			but.Logf("found file %s/%s\n", guid, file)
		}
		if len(missing) == 0 {
			delete(meta.Missing, guid)
		} else {
			meta.Missing[guid] = missing
		}
		UpdateMetadata(rootPath, meta)
	}
}
