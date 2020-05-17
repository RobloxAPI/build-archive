// Update production group.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/anaminus/but"
	"github.com/robloxapi/rbxfetch"
)

var ExpectedFiles = []expectedFile{
	{Name: "API-Dump.json", Method: Client.APIDump},
	{Name: "ClassImages.png", Method: Client.ClassImages},
	{Name: "ReflectionMetadata.xml", Method: Client.ReflectionMetadata},
}

const (
	BuildsDirName    = "builds"
	MetadataFileName = "metadata.json"
	LatestFileName   = "latest.json"
)

var Client = rbxfetch.NewClient()

type expectedFile struct {
	Name   string
	Method func(guid string) (r io.ReadCloser, err error)
}

func FilterBuildType(builds []rbxfetch.Build, types map[string]bool) []rbxfetch.Build {
	bs := builds[:0]
	for _, b := range builds {
		if !types[b.Type] {
			continue
		}
		bs = append(bs, b)
	}
	return bs
}

func FilterBeforeStart(builds []rbxfetch.Build, epoch time.Time) []rbxfetch.Build {
	bs := builds[:0]
	for _, b := range builds {
		if b.Date.Before(epoch) {
			continue
		}
		bs = append(bs, b)
	}
	return bs
}

type Build struct {
	GUID    string
	Date    time.Time
	Version rbxfetch.Version
}

type Metadata struct {
	Files   []string
	Builds  []Build
	Missing map[string][]string
}

// Get content of current metadata file.
func LoadMetadata(root string) (meta Metadata) {
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

// Get list of files that need to be downloaded.
func CheckFiles(root, guid string, meta Metadata) (files []string) {
	missing := meta.Missing[guid]
loop:
	for _, file := range meta.Files {
		path := filepath.Join(root, guid, file)
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			// Skip file that exists.
			continue loop
		}
		for _, m := range missing {
			if m == file {
				// Skip file already known to be missing.
				continue loop
			}
		}
		files = append(files, file)
	}
	return files
}

// Download file from the first successful Location to dstpath.
func FetchFile(guid, dstpath string, method func(string) (io.ReadCloser, error)) error {
	src, err := method(guid)
	if err != nil {
		return fmt.Errorf("fetch file: %w", err)
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

// Write metadata file.
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

// Get expected file Method from file name.
func FindMethod(name string) func(string) (io.ReadCloser, error) {
	for _, file := range ExpectedFiles {
		if file.Name == name {
			return file.Method
		}
	}
	return nil
}

func main() {
	var retry bool
	var configFile string
	flag.BoolVar(&retry, "retry", false, "Attempt to download known missing files.")
	flag.StringVar(&configFile, "config", "", "Configure update client.")
	flag.Parse()

	var config struct {
		Paths struct {
			Root      string
			GroupName string
		}
		StartDate  time.Time
		BuildTypes map[string]bool
		Files      map[string]bool
		rbxfetch.Config
	}

	if configFile != "" {
		b, err := ioutil.ReadFile(configFile)
		but.IfFatal(err, "read config file")
		but.IfFatal(json.Unmarshal(b, &config), "decode config")
	}

	{
		e := ExpectedFiles[:0]
		for _, efile := range ExpectedFiles {
			if config.Files[efile.Name] {
				e = append(e, efile)
			}
		}
		ExpectedFiles = e
	}

	rootPath := filepath.Join(config.Paths.Root, config.Paths.GroupName)
	buildsPath := filepath.Join(rootPath, BuildsDirName)
	but.IfFatal(os.MkdirAll(buildsPath, 0755), "make builds directory")

	// Load metadata.
	meta := LoadMetadata(rootPath)
	meta.Files = make([]string, len(ExpectedFiles))
	for i, file := range ExpectedFiles {
		meta.Files[i] = file.Name
	}
	sort.Strings(meta.Files)
	if meta.Missing == nil {
		meta.Missing = map[string][]string{}
	}

	// Init client.
	Client.CacheMode = rbxfetch.CacheNone
	but.IfFatal(Client.SetConfig(config.Config), "config client")

	// Merge new builds.
	{
		builds, err := Client.Builds()
		but.IfFatal(err, "fetch builds")
		builds = FilterBuildType(builds, config.BuildTypes)
		builds = FilterBeforeStart(builds, config.StartDate)
		type BuildKey struct {
			GUID    string
			Date    int64
			Version rbxfetch.Version
		}
		knownBuilds := map[BuildKey]struct{}{}
		for _, build := range meta.Builds {
			key := BuildKey{
				GUID:    build.GUID,
				Date:    build.Date.Unix(),
				Version: build.Version,
			}
			knownBuilds[key] = struct{}{}
		}
		for _, build := range builds {
			key := BuildKey{
				GUID:    build.GUID,
				Date:    build.Date.Unix(),
				Version: build.Version,
			}
			if _, ok := knownBuilds[key]; ok {
				continue
			}
			meta.Builds = append(meta.Builds, Build{
				GUID:    build.GUID,
				Date:    build.Date,
				Version: build.Version,
			})
		}
		sort.Slice(meta.Builds, func(i, j int) bool {
			return meta.Builds[i].Date.Before(meta.Builds[j].Date)
		})
	}

	// Fetch files.
	for _, build := range meta.Builds {
		files := CheckFiles(buildsPath, build.GUID, meta)
		if len(files) == 0 {
			continue
		}
		path := filepath.Join(buildsPath, build.GUID)
		if err := os.Mkdir(path, 0755); err != nil && !os.IsExist(err) {
			but.IfFatal(err, "make build directory")
		}
		missing := make([]string, 0, len(meta.Files))
		missing = append(missing, meta.Missing[build.GUID]...)
		for _, file := range files {
			err := FetchFile(
				build.GUID,
				filepath.Join(buildsPath, build.GUID, file),
				FindMethod(file),
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
	}

	// Write metadata.
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
			err := FetchFile(
				guid,
				filepath.Join(buildsPath, guid, file),
				FindMethod(file),
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
	}
	UpdateMetadata(rootPath, meta)
}
