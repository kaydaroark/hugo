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
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/gohugoio/hugo/langs"

	"github.com/spf13/afero"
)

const hugoFsMarker = "__hugofs_"

var (
	_ LanguageAnnouncer = (*LanguageFileInfo)(nil)
	_ FilePather        = (*LanguageFileInfo)(nil)
	_ afero.Lstater     = (*LanguageFs)(nil)
)

// LanguageAnnouncer is aware of its language.
type LanguageAnnouncer interface {
	Lang() string
	TranslationBaseName() string
}

// FilePather is aware of its file's location.
type FilePather interface {
	// Filename gets the full path and filename to the file.
	Filename() string

	// Path gets the content relative path including file name and extension.
	// The directory is relative to the content root where "content" is a broad term.
	Path() string

	// RealName is FileInfo.Name in its original form.
	RealName() string

	BaseDir() string
}

// LanguageDirsMerger implements the afero.DirsMerger interface, which is used
// to merge two directories.
// TODO(bep) mod consider reverting this
var LanguageDirsMerger = func(lofi, bofi []os.FileInfo) ([]os.FileInfo, error) {
	m := make(map[string]os.FileInfo)
	nameWeight := func(fi os.FileInfo) (string, int) {
		if fil, ok := fi.(*LanguageFileInfo); ok {
			return fil.virtualName, fil.weight
		}
		return fi.Name(), 0
	}

	for _, fi := range lofi {
		name, _ := nameWeight(fi)
		m[name] = fi
	}

	for _, fi := range bofi {
		name, weight := nameWeight(fi)
		existing, found := m[name]
		var existingWeight int
		if found {
			_, existingWeight = nameWeight(existing)
		}

		if !found || existingWeight < weight {
			m[name] = fi
		}
	}

	merged := make([]os.FileInfo, len(m))
	i := 0
	for _, v := range m {
		merged[i] = v
		i++
	}

	return merged, nil
}

// LanguageFileInfo is a super-set of os.FileInfo with additional information
// about the file in relation to its Hugo language.
type LanguageFileInfo struct {
	os.FileInfo
	lang                string
	baseDir             string
	realFilename        string
	relFilename         string
	name                string
	realName            string
	virtualName         string
	translationBaseName string

	// We add some weight to the files in their own language's content directory.
	weight int
}

// Filename returns a file's real filename including the base (e.g.
// "/my/base/sect/page.md").
func (fi *LanguageFileInfo) Filename() string {
	return fi.realFilename
}

// Path returns a file's filename relative to the base (e.g. "sect/page.md").
func (fi *LanguageFileInfo) Path() string {
	return fi.relFilename
}

// RealName returns a file's real base name (e.g. "page.md").
func (fi *LanguageFileInfo) RealName() string {
	return fi.realName
}

// BaseDir returns a file's base directory (e.g. "/my/base").
func (fi *LanguageFileInfo) BaseDir() string {
	return fi.baseDir
}

// Lang returns a file's language (e.g. "sv").
func (fi *LanguageFileInfo) Lang() string {
	return fi.lang
}

// TranslationBaseName returns the base filename without any extension or language
// identifiers (ie. "page").
func (fi *LanguageFileInfo) TranslationBaseName() string {
	return fi.translationBaseName
}

// Name is the name of the file within this filesystem without any path info.
// It will be marked with language information so we can identify it as ours
// (e.g. "__sv__hugofs_page.md").
func (fi *LanguageFileInfo) Name() string {
	return fi.name
}

type languageFile struct {
	afero.File
	fs *LanguageFs
}

// Readdir creates FileInfo entries by calling Lstat if possible.
func (l *languageFile) Readdir(c int) (ofi []os.FileInfo, err error) {
	names, err := l.File.Readdirnames(c)
	if err != nil {
		return nil, err
	}

	fis := make([]os.FileInfo, len(names))

	for i, name := range names {
		fi, _, err := l.fs.LstatIfPossible(filepath.Join(l.Name(), name))

		if err != nil {
			return nil, err
		}
		fis[i] = fi
	}

	return fis, err
}

// LanguageFs represents a language filesystem.
type LanguageFs struct {
	// This Fs is usually created with a BasePathFs
	basePath string

	lang      string
	languages map[string]bool

	afero.Fs
}

// NewLanguageFs creates a new language filesystem.
func NewLanguageFs(lang string, languages map[string]bool, fs afero.Fs) *LanguageFs {

	var basePath string

	if bfs, ok := fs.(*afero.BasePathFs); ok {
		basePath, _ = bfs.RealPath("")
	}

	return &LanguageFs{lang: lang, languages: languages, basePath: basePath, Fs: fs}
}

func (fs *LanguageFs) getLang(fi os.FileInfo) (string, error) {
	if fs.lang != "" {
		return fs.lang, nil
	}
	if la, ok := fi.(LanguageAnnouncer); ok {
		return la.Lang(), nil
	}

	return "", errors.New("failed to resolve language")

}

func (fs *LanguageFs) marker(lang string) string {
	return "__" + lang + hugoFsMarker
}

// Lang returns a language filesystem's language (e.g. "sv").
// TODO(bep) mod remove
func (fs *LanguageFs) Lang() string {
	return fs.lang
}

// Stat returns the os.FileInfo of a given file.
func (fs *LanguageFs) Stat(name string) (os.FileInfo, error) {
	name, err := fs.realName(name)
	if err != nil {
		return nil, err
	}

	fi, err := fs.Fs.Stat(name)
	if err != nil {
		return nil, err
	}

	return fs.newLanguageFileInfo(name, fi)
}

// Open opens the named file for reading.
func (fs *LanguageFs) Open(name string) (afero.File, error) {
	name, err := fs.realName(name)
	if err != nil {
		return nil, err
	}
	f, err := fs.Fs.Open(name)

	if err != nil {
		return nil, err
	}
	return &languageFile{File: f, fs: fs}, nil
}

// LstatIfPossible returns the os.FileInfo structure describing a given file.
// It attempts to use Lstat if supported or defers to the os.  In addition to
// the FileInfo, a boolean is returned telling whether Lstat was called.
func (fs *LanguageFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	name, err := fs.realName(name)
	if err != nil {
		return nil, false, err
	}

	var fi os.FileInfo
	var b bool

	if lif, ok := fs.Fs.(afero.Lstater); ok {
		fi, b, err = lif.LstatIfPossible(name)
	} else {
		fi, err = fs.Fs.Stat(name)
	}

	if err != nil {
		return nil, b, err
	}

	lfi, err := fs.newLanguageFileInfo(name, fi)

	return lfi, b, err
}

func (fs *LanguageFs) realPath(name string) (string, error) {
	if baseFs, ok := fs.Fs.(*afero.BasePathFs); ok {
		return baseFs.RealPath(name)
	}
	return name, nil
}

func (fs *LanguageFs) realName(name string) (string, error) {
	markerIdx := strings.Index(name, hugoFsMarker)

	if markerIdx != -1 {
		return name[markerIdx+len(hugoFsMarker):], nil
	}

	if fs.basePath == "" {
		return name, nil
	}

	return strings.TrimPrefix(name, fs.basePath), nil
}

func (fs *LanguageFs) newLanguageFileInfo(name string, fi os.FileInfo) (*LanguageFileInfo, error) {
	name = filepath.Clean(name)
	realFilename, err := fs.realPath(name)
	if err != nil {
		return nil, err
	}

	lang, err := fs.getLang(fi)
	if err != nil {
		return nil, err
	}
	nameMarker := fs.marker(lang)

	return newLanguageFileInfo(
		name, realFilename,
		lang, nameMarker, fs.basePath,
		fs.languages, fi)

}

func newLanguageFileInfoFromRealFilenameInfo(rfi RealFilenameInfo, filename, lang string) (*LanguageFileInfo, error) {
	return newLanguageFileInfo(
		filename, rfi.RealFilename(),
		lang, "todo", "todobase",
		langs.Languages{}.AsSet(), rfi,
	)
}

func newLanguageFileInfo(
	filename, realFilename,
	fsLang, nameMarker, basePath string,
	languages map[string]bool,

	fi os.FileInfo) (*LanguageFileInfo, error) {

	filename = filepath.Clean(filename)
	_, name := filepath.Split(filename)

	realName := name
	virtualName := name

	baseNameNoExt := ""
	lang := fsLang
	weight := 1

	if !fi.IsDir() {

		// Try to extract the language from the file name.
		// Any valid language identificator in the name will win over the
		// language set on the file system, e.g. "mypost.en.md".
		baseName := filepath.Base(name)
		ext := filepath.Ext(baseName)
		baseNameNoExt = baseName

		if ext != "" {
			baseNameNoExt = strings.TrimSuffix(baseNameNoExt, ext)
		}

		fileLangExt := filepath.Ext(baseNameNoExt)
		fileLang := strings.TrimPrefix(fileLangExt, ".")

		if languages[fileLang] {
			lang = fileLang

			baseNameNoExt = strings.TrimSuffix(baseNameNoExt, fileLangExt)
		}

		// This connects the filename to the filesystem, not the language.
		virtualName = baseNameNoExt + "." + lang + ext

		name = nameMarker + name
	}

	if lang == fsLang {
		// This file's language belongs in this directory, add some weight to it
		// to make it more important.
		weight++
	}

	if basePath != "" && fi.IsDir() {
		// For directories we always want to start from the union view.
		realFilename = strings.TrimPrefix(realFilename, basePath)
	}

	return &LanguageFileInfo{
		lang:                lang,
		weight:              weight,
		realFilename:        realFilename,
		realName:            realName,
		relFilename:         strings.TrimPrefix(strings.TrimPrefix(realFilename, basePath), string(os.PathSeparator)),
		name:                name,
		virtualName:         virtualName,
		translationBaseName: baseNameNoExt,
		baseDir:             basePath,
		FileInfo:            fi}, nil
}
