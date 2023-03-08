package main

import (
	eria "github.com/project-eria/eria-core"
	"github.com/project-eria/go-wot/dataSchema"
	"github.com/project-eria/go-wot/interaction"
	zlog "github.com/rs/zerolog/log"
)

var config = struct {
	Host        string             `yaml:"host"`
	Port        uint               `yaml:"port" default:"80"`
	ExposedAddr string             `yaml:"exposedAddr"`
	Contexts    map[string]Context `yaml:"contexts"`
}{}

type Context struct {
	Title string      `yaml:"title" required:"true"`
	Desc  string      `yaml:"desc" required:"true"`
	Type  string      `yaml:"type" required:"true"`
	Value interface{} `yaml:"value" required:"true"`
}

func main() {
	defer func() {
		zlog.Info().Msg("[main] Stopped")
	}()

	eria.Init("ERIA Shutter Manager")
	// Loading config
	configManager := eria.LoadConfig(&config)

	eriaServer := eria.NewServer(config.Host, config.Port, config.ExposedAddr, "")

	td, _ := eria.NewThingDescription(
		"eria:manager:context",
		eria.AppVersion,
		"Context",
		"Context Manager",
		[]string{},
	)

	for ref, context := range config.Contexts {
		// TODO different data types
		booleanData := dataSchema.NewBoolean(false)
		property := interaction.NewProperty(
			ref,
			context.Title,
			context.Desc,
			false,
			false,
			true,
			booleanData,
		)
		td.AddProperty(property)
	}

	eriaThing, _ := eriaServer.AddThing("", td)

	for ref := range config.Contexts {
		ref := ref // Copy https://go.dev/doc/faq#closures_and_goroutines
		eriaThing.AddChangeCallBack(ref, func(value interface{}) {
			if context, in := config.Contexts[ref]; in {
				zlog.Trace().Str("property", ref).Interface("value", value).Msg("[main:changeCallBack] Change detected, saving config file")

				// Note: cannot directly assign to struct field config.Contexts[ref].Value in map
				context.Value = value
				config.Contexts[ref] = context
				configManager.Save()
			}
		})
	}
	eriaServer.StartServer()
}
