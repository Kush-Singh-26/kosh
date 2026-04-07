package generators

import (
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// GeneratorOptions holds all common and specific parameters for various site-wide generators.
type GeneratorOptions struct {
	Sink            fspkg.ArtifactSink
	BaseURL         string
	Posts           []models.PostMetadata
	Tags            map[string][]models.PostMetadata
	OutputPath      string
	Title           string
	Description     string
	SiteTitle       string
	SiteDescription string
	GraphConfig     models.GraphConfig
	BuildVersion    int64
	ForceRebuild    bool
	Assets          map[string]string
	IsTesting       bool
}
