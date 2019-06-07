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

const hugoFsMarker = "__hugofs_"

var ()

type LangFilePather interface {
	LanguageAnnouncer
	FilePather
}

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
