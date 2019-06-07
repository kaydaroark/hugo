// Copyright 2019 The Hugo Authors. All rights reserved.
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
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"github.com/spf13/afero"
)

var (
	_ afero.Fs          = (*LingoFs)(nil)
	_ afero.Lstater     = (*LingoFs)(nil)
	_ afero.File        = (*LingoDir)(nil)
	_ FilePather        = (*lingoFileInfo)(nil) // TODO(bep) mods remove (most of) this
	_ LanguageAnnouncer = (*lingoFileInfo)(nil)
)

func NewLingoFs(langs map[string]bool, sources ...MetaFs) (*LingoFs, error) {
	if len(sources) < 2 {
		return nil, errors.New("requires at least 2 filesystems")
	}
	first := sources[0]
	rest := sources[1:]

	common := &lingoFsCommon{
		languages: langs,
	}

	root := &LingoFs{lingoFsCommon: common, source: first}
	root.root = root

	parent := root
	for _, fs := range rest {
		lfs := &LingoFs{lingoFsCommon: common, source: fs, root: root}
		parent.child = lfs
		parent = lfs

	}

	return root, nil
}

type FileOpener interface {
	Open() (afero.File, error)
}

/*

Base top/botton

sv/foo/index.md, bar.sv.txt, sar.en.txt
en/foo/index.md, bar.sv.txt, sar.sv.txt
no/foo/index.md
en/images/image.jpg
no/images/image.jpg

foo.ReadDir => 6 files ? Name "no/

or:

sv/foo.ReadDir index.md bar.sv.txt (sv) sar.sv.txt (en)
en/foo.ReadDir index.md  sar.en.txt (sv)





*/

type LangFsProvider interface {
	Fs() afero.Fs
	Lang() string
}

// TODO(bep) mod dir files same name different languages
type LingoDir struct {
	fs      *LingoFs
	fi      os.FileInfo // TODO(bep) mod remove
	dirname string
}

func (f *LingoDir) Close() error {
	return nil
}

func (f *LingoDir) Name() string {
	panic("not implemented")
}

func (f *LingoDir) Read(p []byte) (n int, err error) {
	panic("not implemented")
}

func (f *LingoDir) ReadAt(p []byte, off int64) (n int, err error) {
	panic("not implemented")
}

func (f *LingoDir) Readdir(count int) ([]os.FileInfo, error) {
	return f.fs.readDirs(f.dirname, count)
}

func (f *LingoDir) Readdirnames(count int) ([]string, error) {
	dirsi, err := f.Readdir(count)
	if err != nil {
		return nil, err
	}

	dirs := make([]string, len(dirsi))
	for i, d := range dirsi {
		dirs[i] = d.Name()
	}
	return dirs, nil
}

func (f *LingoDir) Seek(offset int64, whence int) (int64, error) {
	panic("not implemented")
}

func (f *LingoDir) Stat() (os.FileInfo, error) {
	panic("not implemented")
}

func (f *LingoDir) Sync() error {
	panic("not implemented")
}

func (f *LingoDir) Truncate(size int64) error {
	panic("not implemented")
}

func (f *LingoDir) Write(p []byte) (n int, err error) {
	panic("not implemented")
}

func (f *LingoDir) WriteAt(p []byte, off int64) (n int, err error) {
	panic("not implemented")
}

func (f *LingoDir) WriteString(s string) (ret int, err error) {
	panic("not implemented")
}

type LingoFs struct {
	*lingoFsCommon
	root   *LingoFs
	child  *LingoFs
	source LangFsProvider
}

func (fs *LingoFs) Chmod(n string, m os.FileMode) error {
	return syscall.EPERM
}

func (fs *LingoFs) Chtimes(n string, a, m time.Time) error {
	return syscall.EPERM
}

// TODO(bep) mod lstat
func (fs *LingoFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	fi, _, err := fs.pickFirst(name)
	if err != nil {
		return nil, false, err
	}
	if fi.IsDir() {
		return fs.newDirOpener(name, fi), false, nil
	}

	return nil, false, errors.Errorf("lstat: files not supported: %q", name)

}

func (fs *LingoFs) Mkdir(n string, p os.FileMode) error {
	return syscall.EPERM
}

func (fs *LingoFs) MkdirAll(n string, p os.FileMode) error {
	return syscall.EPERM
}

func (fs *LingoFs) Name() string {
	return "WeightedFileSystem"
}

func (fs *LingoFs) Open(name string) (afero.File, error) {
	fi, lfs, err := fs.pickFirst(name)
	if err != nil {
		return nil, err
	}

	if !fi.IsDir() {
		panic("currently only dirs in here")
	}

	return &LingoDir{
		fs:      lfs,
		fi:      fi,
		dirname: name,
	}, nil

}

func (fs *LingoFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	panic("not implemented")
}

func (fs *LingoFs) ReadDir(name string) ([]os.FileInfo, error) {
	panic("not implemented")
}

func (fs *LingoFs) Remove(n string) error {
	return syscall.EPERM
}

func (fs *LingoFs) RemoveAll(p string) error {
	return syscall.EPERM
}

func (fs *LingoFs) Rename(o, n string) error {
	return syscall.EPERM
}

func (fs *LingoFs) Stat(name string) (os.FileInfo, error) {
	fi, _, err := fs.LstatIfPossible(name)
	return fi, err
}

func (fs *LingoFs) Create(n string) (afero.File, error) {
	return nil, syscall.EPERM
}

func (fs *LingoFs) newDirOpener(name string, fi os.FileInfo) fileOpener {
	return fileOpener{
		FileInfo: fi,
		openFileFunc: func() (afero.File, error) {
			return fs.Open(name)
		},
	}
}

func (fs *LingoFs) applyMeta(name string, fis []os.FileInfo) []os.FileInfo {
	fisn := make([]os.FileInfo, len(fis))
	for i, fi := range fis {
		if fi.IsDir() {
			fisn[i] = fs.root.newDirOpener(filepath.Join(name, fi.Name()), fi)
			continue
		}

		lang := fs.source.Lang()
		fileLang, translationBaseName := fs.langInfoFrom(fi.Name())
		weight := 0
		if fileLang != "" {
			weight = 1
			if fileLang == lang {
				// Give priority to myfile.sv.txt inside the sv filesystem.
				weight++
			}
			lang = fileLang
		}

		var (
			filename string
			baseDir  string
			path     string
		)

		if vfi, ok := fi.(VirtualFileInfo); ok {
			baseDir = vfi.VirtualRoot()
			path = strings.TrimPrefix(filename, baseDir)
		}

		if rfi, ok := fi.(RealFilenameInfo); ok {
			filename = rfi.RealFilename()
		}

		fisn[i] = &lingoFileInfo{
			FileInfo:            fi,
			lang:                lang,
			weight:              weight,
			translationBaseName: translationBaseName,

			filename: filename,
			path:     path,
			baseDir:  baseDir,

			openFileFunc: func() (afero.File, error) {
				return fs.source.Fs().Open(filepath.Join(name, fi.Name()))
			},
		}
	}

	return fisn
}

func (fs *LingoFs) collectFileInfos(root *LingoFs, name string) ([]os.FileInfo, error) {
	var fis []os.FileInfo
	current := root
	for current != nil {
		fi, err := current.source.Fs().Stat(name)
		if err == nil {
			// Gotta match!
			fis = append(fis, fi)
		} else if !os.IsNotExist(err) {
			// Real error
			return nil, err
		}

		// Continue
		current = current.child

	}

	return fis, nil
}

func (fs *LingoFs) filterDuplicates(fis []os.FileInfo) []os.FileInfo {
	type idxWeight struct {
		idx    int
		weight int
	}

	keep := make(map[string]idxWeight)

	for i, fi := range fis {
		if fi.IsDir() {
			continue
		}
		lfi := fi.(*lingoFileInfo)
		if lfi.weight > 0 {
			name := fi.Name()
			k, found := keep[name]
			if !found || lfi.weight > k.weight {
				keep[name] = idxWeight{
					idx:    i,
					weight: lfi.weight,
				}
			}
		}
	}

	if len(keep) > 0 {
		toRemove := make(map[int]bool)
		for i, fi := range fis {
			if fi.IsDir() {
				continue
			}
			k, found := keep[fi.Name()]
			if found && i != k.idx {
				toRemove[i] = true
			}
		}

		filtered := fis[:0]
		for i, fi := range fis {
			if !toRemove[i] {
				filtered = append(filtered, fi)
			}
		}
		fis = filtered
	}

	return fis
}

func (fs *LingoFs) pickFirst(name string) (os.FileInfo, *LingoFs, error) {
	current := fs
	for current != nil {
		fi, err := current.source.Fs().Stat(name)
		if err == nil {
			// Gotta match!
			return fi, current, nil
		}

		if !os.IsNotExist(err) {
			// Real error
			return nil, nil, err
		}

		// Continue
		current = current.child

	}

	// Not found
	return nil, nil, os.ErrNotExist
}

func (fs *LingoFs) readDirs(name string, count int) ([]os.FileInfo, error) {

	collect := func(current *LingoFs) ([]os.FileInfo, error) {
		d, err := current.source.Fs().Open(name)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
			return nil, nil
		} else {
			defer d.Close()
			dirs, err := d.Readdir(-1)
			if err != nil {
				return nil, err
			}
			return current.applyMeta(name, dirs), nil
		}
	}

	var dirs []os.FileInfo

	current := fs
	for current != nil {

		fis, err := collect(current)
		if err != nil {
			return nil, err
		}

		dirs = append(dirs, fis...)
		if count > 0 && len(dirs) >= count {
			return dirs[:count], nil
		}

		current = current.child

	}

	return fs.filterDuplicates(dirs), nil

}

// MetaFs wraps a afero.Fs with some metadata about the Fs.
// TODO(bep) remove this
type MetaFs struct {
	TheFs afero.Fs

	TheLang string
}

func (m MetaFs) Fs() afero.Fs {
	return m.TheFs
}

func (m MetaFs) Lang() string {
	return m.TheLang
}

type fileOpener struct {
	os.FileInfo
	openFileFunc
}

// TODO(bep) mods names, names, names
type lingoFileInfo struct {
	os.FileInfo

	lang                string
	translationBaseName string

	filename string // the real filename in the source filesystem
	baseDir  string
	path     string

	openFileFunc

	// Set when there is language information in the filename.
	weight int
}

func (fi lingoFileInfo) BaseDir() string {
	return fi.baseDir
}

func (fi lingoFileInfo) Filename() string {
	return fi.filename
}

func (fi lingoFileInfo) Lang() string {
	return fi.lang
}

func (fi lingoFileInfo) Path() string {
	return fi.path
}

func (fi lingoFileInfo) RealName() string {
	panic("remove me")
}

// TranslationBaseName returns the base filename without any language
// or file extension.
// E.g. myarticle.en.md becomes myarticle.
func (fi lingoFileInfo) TranslationBaseName() string {
	return fi.translationBaseName
}

type lingoFsCommon struct {
	languages map[string]bool
}

// Try to extract the language from the given filename.
// Any valid language identificator in the name will win over the
// language set on the file system, e.g. "mypost.en.md".
func (l *lingoFsCommon) langInfoFrom(name string) (string, string) {
	var lang string

	baseName := filepath.Base(name)
	ext := filepath.Ext(baseName)
	translationBaseName := baseName

	if ext != "" {
		translationBaseName = strings.TrimSuffix(translationBaseName, ext)
	}

	fileLangExt := filepath.Ext(translationBaseName)
	fileLang := strings.TrimPrefix(fileLangExt, ".")

	if l.languages[fileLang] {
		lang = fileLang

		translationBaseName = strings.TrimSuffix(translationBaseName, fileLangExt)
	}

	return lang, translationBaseName

}

type openFileFunc func() (afero.File, error)

func (f openFileFunc) Open() (afero.File, error) {
	return f()
}
