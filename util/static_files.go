package util

import (
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type StaticFile struct {
	ContentType  string
	LastModified string
	Content      []byte
}

const StaticRouteTemplate = "/static/*"
const StaticUrlPrefix = "/static"

var hashedPathsByFilename map[string]string
var files map[string]*StaticFile

var contentTypesByExt = map[string]string{
	".avif": "image/avif",
	".css":  "text/css; charset=utf-8",
	".gif":  "image/gif",
	".htm":  "text/html; charset=utf-8",
	".html": "text/html; charset=utf-8",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".js":   "text/javascript; charset=utf-8",
	".json": "application/json",
	".mjs":  "text/javascript; charset=utf-8",
	".pdf":  "application/pdf",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".txt":  "text/plain",
	".wasm": "application/wasm",
	".webp": "image/webp",
	".xml":  "text/xml; charset=utf-8",
}

func init() {
	hashedPathsByFilename = make(map[string]string)
	files = make(map[string]*StaticFile)

	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	rootDir := cwd
findRoot:
	for {
		entries, err := os.ReadDir(rootDir)
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			if (entry.Type()&os.ModeDir == 0) && entry.Name() == "Procfile" {
				break findRoot
			}
		}
		parentDir, curDirName := filepath.Split(rootDir)
		if curDirName == "" {
			panic("couldn't find root dir")
		} else {
			rootDir = strings.TrimRight(parentDir, string(filepath.Separator))
		}
	}

	staticDir := path.Join(rootDir, "static")
	dirEntries, err := os.ReadDir(staticDir)
	if err != nil {
		panic(err)
	}

	type FilePathName struct {
		Path string
		Name string
	}

	for _, dirEntry := range dirEntries {
		childPath := path.Join(staticDir, dirEntry.Name())
		stat, err := os.Stat(childPath)
		if err != nil {
			panic(err)
		}
		var filePathNames []FilePathName
		if stat.IsDir() {
			childEntries, err := os.ReadDir(childPath)
			if err != nil {
				panic(err)
			}
			for _, childEntry := range childEntries {
				filePathNames = append(filePathNames, FilePathName{
					Path: path.Join(childPath, childEntry.Name()),
					Name: dirEntry.Name() + "/" + childEntry.Name(),
				})
			}
		} else {
			filePathNames = []FilePathName{{
				Path: childPath,
				Name: dirEntry.Name(),
			}}
		}

		for _, filePathName := range filePathNames {
			lastModified := stat.ModTime().UTC().Format(http.TimeFormat)

			content, err := os.ReadFile(filePathName.Path)
			if err != nil {
				panic(err)
			}

			hasher := fnv.New32a()
			hasher.Write(content)
			hash := hasher.Sum32()
			ext := path.Ext(filePathName.Path)

			urlPath := fmt.Sprintf("%s/%s", StaticUrlPrefix, filePathName.Name)
			hashedPath := fmt.Sprintf("%s.%08x%s", urlPath[:len(urlPath)-len(ext)], hash, ext)

			mimeType, ok := contentTypesByExt[ext]
			if !ok {
				panic(fmt.Errorf("extension doesn't have mime type: %s", ext))
			}

			hashedPathsByFilename[filePathName.Name] = hashedPath
			files[hashedPath] = &StaticFile{
				ContentType:  mimeType,
				LastModified: lastModified,
				Content:      content,
			}
		}
	}
}

func StaticHashedPath(filename string) (string, error) {
	if hashedPath, ok := hashedPathsByFilename[filename]; ok {
		return hashedPath, nil
	}

	return "", fmt.Errorf("static file not found: %q", filename)
}

func GetStaticFile(hashedPath string) (*StaticFile, error) {
	if containsDotDot(hashedPath) {
		return nil, fmt.Errorf("path contains '..': %q", hashedPath)
	}

	if file, ok := files[hashedPath]; ok {
		return file, nil
	}

	return nil, fmt.Errorf("static path not found: %q", hashedPath)
}

func containsDotDot(v string) bool {
	if !strings.Contains(v, "..") {
		return false
	}
	for _, ent := range strings.FieldsFunc(v, isSlashRune) {
		if ent == ".." {
			return true
		}
	}
	return false
}

func isSlashRune(r rune) bool { return r == '/' || r == '\\' }
