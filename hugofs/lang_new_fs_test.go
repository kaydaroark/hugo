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
	"testing"

	"github.com/gohugoio/hugo/langs"
	"github.com/spf13/viper"

	"github.com/spf13/afero"

	"github.com/stretchr/testify/require"
)

// TODO(bep) mod
// tc-lib-color/class-Com.Tecnick.Color.Css and class-Com.Tecnick.Color.sv.Css
func TestLingoFs(t *testing.T) {
	assert := require.New(t)
	v := viper.New()
	v.Set("contentDir", "content")

	langSet := langs.Languages{
		langs.NewLanguage("en", v),
		langs.NewLanguage("sv", v),
	}.AsSet()

	enFs := MetaFs{TheLang: "en", TheFs: afero.NewMemMapFs()}
	svFs := MetaFs{TheLang: "sv", TheFs: afero.NewMemMapFs()}

	for _, fs := range []MetaFs{enFs, svFs} {
		assert.NoError(afero.WriteFile(fs.Fs(), filepath.FromSlash("blog/a.txt"), []byte("abc"), 0777))

		for _, fs2 := range []MetaFs{enFs, svFs} {
			lingoName := fmt.Sprintf("lingo.%s.txt", fs2.TheLang)
			assert.NoError(afero.WriteFile(fs.Fs(), filepath.FromSlash("blog/"+lingoName), []byte(lingoName), 0777))
		}

	}

	lfs, err := NewLingoFs(langSet, enFs, svFs)
	assert.NoError(err)

	afero.Walk(lfs, "", func(path string, info os.FileInfo, err error) error {
		//fmt.Println(":::", path)

		return nil
	})

	blog, err := lfs.Open("blog")
	assert.NoError(err)

	names, err := blog.Readdirnames(-1)
	assert.NoError(err)
	assert.Equal(4, len(names), names)
	assert.Equal([]string{"a.txt", "lingo.en.txt", "a.txt", "lingo.sv.txt"}, names)

	fis, err := blog.Readdir(-1)
	assert.NoError(err)
	assert.Equal(4, len(fis))
	enFi, ok1 := fis[0].(LanguageAnnouncer)
	svFi, ok2 := fis[2].(LanguageAnnouncer)
	assert.True(ok1)
	assert.True(ok2)
	assert.Equal("en", enFi.Lang())
	assert.Equal("sv", svFi.Lang())

}
