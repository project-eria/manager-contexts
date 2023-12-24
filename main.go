package main

import (
	"encoding/json"
	"flag"

	"github.com/gookit/goutil/fsutil"
	"github.com/project-eria/go-wot/thing"

	eria "github.com/project-eria/eria-core"
	"github.com/project-eria/go-wot/dataSchema"
	"github.com/project-eria/go-wot/interaction"
	"github.com/project-eria/go-wot/producer"
	zlog "github.com/rs/zerolog/log"
	"golang.org/x/exp/slices"
)

var config = struct{}{}

type StateData struct {
	Contexts []string `json:"contexts"`
}

var (
	_dataPath  *string
	_stateData StateData
)

func main() {
	defer func() {
		zlog.Info().Msg("[main] Stopped")
	}()
	// (Config flags should be placed before Init)
	_dataPath = flag.String("data-path", "data.json", "state data file path")
	eria.Init("ERIA Contexts Manager")
	// Loading config
	eria.LoadConfig(&config)
	if fsutil.FileExists(*_dataPath) {
		// Loading state data from json file
		zlog.Info().Str("file", *_dataPath).Msg("[main] loading state data, from file")
		jsonData := fsutil.ReadFile(*_dataPath)
		err := json.Unmarshal([]byte(jsonData), &_stateData)
		if err != nil {
			zlog.Warn().Err(err).Msg("[main] Could not unmarshal json")
		}
	} else {
		zlog.Info().Str("file", *_dataPath).Msg("[main] state data file not found, creating it")
		_stateData.Contexts = []string{}
	}

	td := setThing()
	eriaThing, _ := eria.Producer("").AddThing("", td)

	setHandlers(eriaThing)
	eria.Start("")
}

func saveStateData() {
	jsonData, _ := json.Marshal(_stateData)
	err := fsutil.WriteFile(*_dataPath, jsonData, 0644)
	if err != nil {
		zlog.Warn().Err(err).Msg("[main] Could not save state data in json file")
	}
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
		zlog.Trace().Str("property", "actives").Interface("value", _stateData.Contexts).Msg("[main:propertyReadHandler] Value get")
		return _stateData.Contexts, nil
	})

	eriaThing.SetPropertyReadHandler("isActive", func(t producer.ExposedThing, name string, parameters map[string]interface{}) (interface{}, error) {
		// The presence and the value type of the `context` options has been checked by Protocol Binding
		return slices.Contains(_stateData.Contexts, parameters["context"].(string)), nil
	})

	eriaThing.SetPropertyObserveHandler("isActive", func(t producer.ExposedThing, name string, parameters map[string]interface{}) (interface{}, error) {
		// The presence and the value type of the `context` options has been checked by Protocol Binding
		return slices.Contains(_stateData.Contexts, parameters["context"].(string)), nil
	})

	eriaThing.SetObserverSelectorHandler("isActive", func(emitOptions map[string]interface{}, listenerOptions map[string]interface{}) bool {
		return emitOptions["context"] == listenerOptions["context"]
	})

	eriaThing.SetActionHandler("setContext", func(value interface{}, parameters map[string]interface{}) (interface{}, error) {
		context := value.(string)
		i := slices.Index(_stateData.Contexts, context)
		if i == -1 {
			// Not in array yet
			_stateData.Contexts = append(_stateData.Contexts, context)
			saveStateData()
			eriaThing.EmitPropertyChange("actives", _stateData.Contexts, map[string]interface{}{"context": context})
			eriaThing.EmitPropertyChange("isActive", true, map[string]interface{}{"context": context})
			return true, nil
		}
		return false, nil // Nothing was done
	})

	eriaThing.SetActionHandler("unsetContext", func(value interface{}, parameters map[string]interface{}) (interface{}, error) {
		context := value.(string)
		i := slices.Index(_stateData.Contexts, context)
		if i != -1 {
			// In array
			_stateData.Contexts = slices.Delete(_stateData.Contexts, i, i+1)
			saveStateData()
			eriaThing.EmitPropertyChange("actives", _stateData.Contexts, map[string]interface{}{"context": context})
			eriaThing.EmitPropertyChange("isActive", false, map[string]interface{}{"context": context})
			return true, nil
		}
		return false, nil // Nothing was done
	})
}
