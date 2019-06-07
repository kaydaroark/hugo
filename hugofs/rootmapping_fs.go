// Copyright 2018 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hugofs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gohugoio/hugo/langs"

	radix "github.com/hashicorp/go-immutable-radix"
	"github.com/spf13/afero"
)

var filepathSeparator = string(filepath.Separator)

// A RootMappingFs maps several roots into one. Note that the root of this filesystem
// is directories only, and they will be returned in Readdir and Readdirnames
// in the order given.
type RootMappingFs struct {
	afero.Fs
	rootMapToReal *radix.Node
	virtualRoots  []string
}

type rootMappingFile struct {
	afero.File
	fs   *RootMappingFs
	name string
	rm   RootMapping
}

type rootMappingFileInfo struct {
	name string
}

func (fi *rootMappingFileInfo) Name() string {
	return fi.name
}

func (fi *rootMappingFileInfo) Size() int64 {
	panic("not implemented")
}

func (fi *rootMappingFileInfo) Mode() os.FileMode {
	return os.ModeDir
}

func (fi *rootMappingFileInfo) ModTime() time.Time {
	panic("not implemented")
}

func (fi *rootMappingFileInfo) IsDir() bool {
	return true
}

func (fi *rootMappingFileInfo) Sys() interface{} {
	return nil
}

func newRootMappingDirFileInfo(name string) *rootMappingFileInfo {
	return &rootMappingFileInfo{name: name}
}

type RootMapping struct {
	From string
	To   string

	// Metadata
	Lang string
}

func (rm *RootMapping) clean() {
	rm.From = filepath.Clean(rm.From)
	rm.To = filepath.Clean(rm.To)
}

// NewRootMappingFs creates a new RootMappingFs on top of the provided with
// of root mappings with some optional metadata about the root.
// Note that 'From' represents a virtual root that maps to the actual filename in 'To'.
func NewRootMappingFs(fs afero.Fs, rms ...RootMapping) (*RootMappingFs, error) {
	rootMapToReal := radix.New().Txn()
	var virtualRoots []string

	for _, rm := range rms {
		(&rm).clean()

		// We need to preserve the original order for Readdir
		virtualRoots = append(virtualRoots, rm.From)

		rootMapToReal.Insert([]byte(rm.From), rm)
	}

	if rfs, ok := fs.(*afero.BasePathFs); ok {
		fs = NewBasePathRealFilenameFs(rfs)
	}

	return &RootMappingFs{Fs: fs,
		virtualRoots:  virtualRoots,
		rootMapToReal: rootMapToReal.Commit().Root()}, nil
}

// NewRootMappingFsFromFromTo is a convenicence variant of NewRootMappingFs taking
// From and To as string pairs.
func NewRootMappingFsFromFromTo(fs afero.Fs, fromTo ...string) (*RootMappingFs, error) {
	rms := make([]RootMapping, len(fromTo)/2)
	for i, j := 0, 0; j < len(fromTo); i, j = i+1, j+2 {
		rms[i] = RootMapping{
			From: fromTo[j],
			To:   fromTo[j+1],
		}
	}

	return NewRootMappingFs(fs, rms...)
}

// Stat returns the os.FileInfo structure describing a given file.  If there is
// an error, it will be of type *os.PathError.
func (fs *RootMappingFs) Stat(name string) (os.FileInfo, error) {

	if fs.isRoot(name) {
		return newRootMappingDirFileInfo(name), nil
	}
	realName, _, rm := fs.realNameAndRoot(name)

	fi, err := fs.Fs.Stat(realName)
	if err != nil {
		return nil, err
	}

	var (
		filename string
		root     string
	)
	if rfi, ok := fi.(RealFilenameInfo); ok {
		filename = rfi.RealFilename()
	}

	if vfi, ok := fi.(VirtualFileInfo); ok {
		root = vfi.VirtualRoot()
	}

	return decorateFileInfo(fi, filename, root, rm.Lang), nil

}

func decorateFileInfo(fi os.FileInfo, filename, root, lang string) os.FileInfo {
	if lang == "" {
		return &realFilenameInfo{
			FileInfo: fi,
			filename: filename,
			root:     root,
		}
	}

	return &realFilenameAndLangInfo{
		FileInfo: fi,
		filename: filename,
		root:     root,
		lang:     lang,
	}
}

func (fs *RootMappingFs) isRoot(name string) bool {
	return name == "" || name == filepathSeparator

}

// Open opens the named file for reading.
func (fs *RootMappingFs) Open(name string) (afero.File, error) {
	if fs.isRoot(name) {
		return &rootMappingFile{name: name, fs: fs}, nil
	}
	realName, _, rm := fs.realNameAndRoot(name)
	f, err := fs.Fs.Open(realName)
	if err != nil {
		return nil, err
	}
	return &rootMappingFile{File: f, name: name, fs: fs, rm: rm}, nil
}

// LstatIfPossible returns the os.FileInfo structure describing a given file.
// It attempts to use Lstat if supported or defers to the os.  In addition to
// the FileInfo, a boolean is returned telling whether Lstat was called.
func (fs *RootMappingFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {

	if fs.isRoot(name) {
		return newRootMappingDirFileInfo(name), false, nil
	}

	name, foo, rm := fs.realNameAndRoot(name)
	fmt.Println("RN", name, foo)

	if ls, ok := fs.Fs.(afero.Lstater); ok {
		fi, b, err := ls.LstatIfPossible(name)
		if err != nil {
			return nil, b, err
		}

		// TODO(bep) mod
		return decorateFileInfo(fi, name, "", rm.Lang), b, nil

	}

	fi, err := fs.Stat(name)
	return fi, false, err
}

func (fs *RootMappingFs) realNameAndRoot(name string) (string, string, RootMapping) {
	key, val, found := fs.rootMapToReal.LongestPrefix([]byte(filepath.Clean(name)))
	if !found {
		return name, "", RootMapping{}
	}
	keystr := string(key)

	rm := val.(RootMapping)
	filename := filepath.Join(rm.To, strings.TrimPrefix(name, keystr))

	return filename, keystr, rm
}

func (f *rootMappingFile) Readdir(count int) ([]os.FileInfo, error) {
	if f.File == nil {
		dirsn := make([]os.FileInfo, 0)
		for i := 0; i < len(f.fs.virtualRoots); i++ {
			if count != -1 && i >= count {
				break
			}
			dirsn = append(dirsn, newRootMappingDirFileInfo(f.fs.virtualRoots[i]))
		}
		return dirsn, nil
	}

	fis, err := f.File.Readdir(count)
	if err != nil {
		return nil, err
	}

	if f.rm.Lang == "" {
		return fis, nil
	}

	// Add language information to FileInfo

	fisn := make([]os.FileInfo, len(fis))
	for i, fi := range fis {

		rfi := fi.(*realFilenameInfo)

		lfi, err := newLanguageFileInfo(
			fi.Name(), rfi.RealFilename(),
			f.rm.Lang, "", rfi.VirtualRoot(),
			langs.Languages{}.AsSet(), fi)

		if err != nil {
			return nil, err
		}
		fisn[i] = lfi
	}

	return fisn, nil

}

func (f *rootMappingFile) Readdirnames(count int) ([]string, error) {
	dirs, err := f.Readdir(count)
	if err != nil {
		return nil, err
	}
	dirss := make([]string, len(dirs))
	for i, d := range dirs {
		dirss[i] = d.Name()
	}
	return dirss, nil
}

func (f *rootMappingFile) Name() string {
	return f.name
}

func (f *rootMappingFile) Close() error {
	if f.File == nil {
		return nil
	}
	return f.File.Close()
}
