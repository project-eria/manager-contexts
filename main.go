package main

import (
	"github.com/project-eria/go-wot/thing"

	eria "github.com/project-eria/eria-core"
	"github.com/project-eria/go-wot/dataSchema"
	"github.com/project-eria/go-wot/interaction"
	"github.com/project-eria/go-wot/producer"
	zlog "github.com/rs/zerolog/log"
	"golang.org/x/exp/slices"
)

var config = struct {
	Contexts []string `yaml:"contexts" required:"true"`
}{}

func main() {
	defer func() {
		zlog.Info().Msg("[main] Stopped")
	}()
	eria.Init("ERIA Contexts Manager")
	// Loading config
	eria.LoadConfig(&config)
	td := setThing()
	eriaThing, _ := eria.Producer("").AddThing("", td)

	setHandlers(eriaThing)
	eria.Start("")
}

func setThing() *thing.Thing {
	td, _ := eria.NewThingDescription(
		"eria:manager:context",
		eria.AppVersion,
		"Context",
		"Context Manager",
		[]string{},
	)

	activesData := dataSchema.NewArray([]string{}, 0, 0)
	activesProperty := interaction.NewProperty(
		"actives",
		"Actives Contexts",
		"List of contexts that are currently actives",
		true,
		false,
		true,
		nil,
		activesData,
	)
	td.AddProperty(activesProperty)

	isActiveData := dataSchema.NewBoolean(false)
	isActiveProperty := interaction.NewProperty(
		"isActive",
		"Is context active",
		"Tell if a specific context is active",
		true,
		false,
		true,
		map[string]dataSchema.Data{
			"context": dataSchema.NewString("", 0, 0, ""),
		},
		isActiveData,
	)
	td.AddProperty(isActiveProperty)

	setContextInputData := dataSchema.NewString("", 0, 0, "")
	setContextOutputData := dataSchema.NewBoolean(false)
	setContext := interaction.NewAction(
		"setContext",
		"Set context",
		"Set the context as active",
		&setContextInputData,
		&setContextOutputData,
	)
	td.AddAction(setContext)

	unsetContextInputData := dataSchema.NewString("", 0, 0, "")
	unsetContextOutputData := dataSchema.NewBoolean(false)
	unsetContext := interaction.NewAction(
		"unsetContext",
		"Unset context",
		"Set the context as inactive",
		&unsetContextInputData,
		&unsetContextOutputData,
	)
	td.AddAction(unsetContext)
	return td
}

func setHandlers(eriaThing producer.ExposedThing) {
	eriaThing.SetPropertyReadHandler("actives", func(t producer.ExposedThing, name string, parameters map[string]interface{}) (interface{}, error) {
		zlog.Trace().Str("property", "actives").Interface("value", config.Contexts).Msg("[main:propertyReadHandler] Value get")
		return config.Contexts, nil
	})

	eriaThing.SetPropertyReadHandler("isActive", func(t producer.ExposedThing, name string, parameters map[string]interface{}) (interface{}, error) {
		// The presence and the value type of the `context` options has been checked by Protocol Binding
		return slices.Contains(config.Contexts, parameters["context"].(string)), nil
	})

	eriaThing.SetPropertyObserveHandler("isActive", func(t producer.ExposedThing, name string, parameters map[string]interface{}) (interface{}, error) {
		// The presence and the value type of the `context` options has been checked by Protocol Binding
		return slices.Contains(config.Contexts, parameters["context"].(string)), nil
	})

	eriaThing.SetObserverSelectorHandler("isActive", func(emitOptions map[string]interface{}, listenerOptions map[string]interface{}) bool {
		return emitOptions["context"] == listenerOptions["context"]
	})

	eriaThing.SetActionHandler("setContext", func(value interface{}, parameters map[string]interface{}) (interface{}, error) {
		context := value.(string)
		i := slices.Index(config.Contexts, context)
		if i == -1 {
			// Not in array yet
			config.Contexts = append(config.Contexts, context)
			eriaThing.EmitPropertyChange("actives", config.Contexts, map[string]interface{}{"context": context})
			eriaThing.EmitPropertyChange("isActive", true, map[string]interface{}{"context": context})
			return true, nil
		}
		return false, nil // Nothing was done
	})

	eriaThing.SetActionHandler("unsetContext", func(value interface{}, parameters map[string]interface{}) (interface{}, error) {
		context := value.(string)
		i := slices.Index(config.Contexts, context)
		if i != -1 {
			// In array
			config.Contexts = slices.Delete(config.Contexts, i, i+1)
			eriaThing.EmitPropertyChange("actives", config.Contexts, map[string]interface{}{"context": context})
			eriaThing.EmitPropertyChange("isActive", false, map[string]interface{}{"context": context})
			return true, nil
		}
		return false, nil // Nothing was done
	})
}
