package main

import (
	"encoding/json"
	"flag"
	"manager-context/lib"
	"time"

	"github.com/go-co-op/gocron/v2"
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
	eria.Init("ERIA Contexts Manager", &config)
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

	// Set week/day context
	scheduler := eria.GetCronScheduler()
	// Update daily contexts each morning at 0:00
	scheduler.NewJob(
		gocron.DailyJob(
			1,
			gocron.NewAtTimes(
				gocron.NewAtTime(0, 0, 0),
			),
		),
		gocron.NewTask(setDailyContexts, eriaThing),
		gocron.WithTags("refresh", "main"),
		gocron.WithStartAt(
			gocron.WithStartImmediately(),
		),
	)
	eria.Start("")
}

func setDailyContexts(eriaThing producer.ExposedThing) {
	now := time.Now().In(eria.Location())

	current, removed, added := lib.GetDailyContexts(now, _stateData.Contexts)
	_stateData.Contexts = current
	saveStateData()

	// Emit the change for removed contexts
	// Excepts([]string{"a", "c"}, []string{"a", "b"},...) => []string{"c"}
	for _, context := range removed {
		eriaThing.EmitPropertyChange("isActive", false, map[string]interface{}{"context": context})
	}
	// Emit the change for added contexts
	for _, context := range added {
		eriaThing.EmitPropertyChange("isActive", true, map[string]interface{}{"context": context})
	}

	eriaThing.EmitPropertyChange("actives", _stateData.Contexts, map[string]interface{}{})
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

	activesData, _ := dataSchema.NewArray()
	activesProperty := interaction.NewProperty(
		"actives",
		"Actives Contexts",
		"List of contexts that are currently actives",
		activesData,
		interaction.PropertyReadOnly(true),
	)
	td.AddProperty(activesProperty)

	isActiveData, _ := dataSchema.NewBoolean()
	uriContext, _ := dataSchema.NewString()
	isActiveProperty := interaction.NewProperty(
		"isActive",
		"Is context active",
		"Tell if a specific context is active",
		isActiveData,
		interaction.PropertyReadOnly(true),
		interaction.PropertyUriVariable("context", uriContext),
	)
	td.AddProperty(isActiveProperty)

	setContextInputData, _ := dataSchema.NewString()
	setContextOutputData, _ := dataSchema.NewBoolean()
	setContext := interaction.NewAction(
		"setContext",
		"Set context",
		"Set the context as active",
		interaction.ActionInput(&setContextInputData),
		interaction.ActionOutput(&setContextOutputData),
	)
	td.AddAction(setContext)

	unsetContextInputData, _ := dataSchema.NewString()
	unsetContextOutputData, _ := dataSchema.NewBoolean()
	unsetContext := interaction.NewAction(
		"unsetContext",
		"Unset context",
		"Set the context as inactive",
		interaction.ActionInput(&unsetContextInputData),
		interaction.ActionOutput(&unsetContextOutputData),
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
