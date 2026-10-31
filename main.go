package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"manager-context/lib"
	"net/http"
	"os"
	"strings"
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

// Context is a context people choose (a state of the house: away, guests…), with its name.
type Context struct {
	ID   string `yaml:"id" required:"true"`
	Name string `yaml:"name"`
}

var config = struct {
	Contexts []Context `yaml:"contexts,omitempty"` // contexts people choose, in display order
	// Things channel of eria-auth (http://auth:8444): the active contexts are
	// the states of the house that restrict the rights (AUTH-DESIGN §2.5).
	AuthChannel string `yaml:"authChannel,omitempty"`
}{}

type StateData struct {
	Contexts []string `json:"contexts"`
}

var (
	_dataPath        *string
	_defaultDataPath = "data.json"
	_stateData       StateData
)

func main() {
	defer func() {
		zlog.Info().Msg("[main] Stopped")
	}()

	// (Config flags should be placed before Init)
	envDataPath, ok := os.LookupEnv("ERIA_DATA_PATH")
	if ok {
		_defaultDataPath = envDataPath
	}
	_dataPath = flag.String("data-path", _defaultDataPath, "state data file path")

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
	// Again every minute: auth converges after a restart or a restore.
	go func() {
		for range time.Tick(time.Minute) {
			// ponytail: read without a lock, like the property handlers
			publishStates(slices.Clone(_stateData.Contexts))
		}
	}()
	eria.Start("")
}

// publishStates sends the active contexts to auth, which keeps the ones it
// knows as states. ponytail: unsigned until the app keys (AUTH-DESIGN §4.7
// step 3); a state only restricts.
func publishStates(contexts []string) {
	if config.AuthChannel == "" {
		return
	}
	body, _ := json.Marshal(contexts)
	req, _ := http.NewRequest(http.MethodPut, strings.TrimSuffix(config.AuthChannel, "/")+"/v1/states", bytes.NewReader(body))
	resp, err := authClient.Do(req)
	if err != nil {
		zlog.Warn().Err(err).Msg("[main:publishStates] auth unreachable")
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		zlog.Warn().Int("status", resp.StatusCode).Msg("[main:publishStates] refused by auth")
	}
}

var authClient = &http.Client{Timeout: 5 * time.Second}

func setDailyContexts(eriaThing producer.ExposedThing) {
	now := time.Now().In(eria.Location())

	current, removed, added := lib.GetDailyContexts(now, _stateData.Contexts)
	applyContexts(eriaThing, current, removed, added)
	eriaThing.EmitPropertyChange("chosen", chosenContexts(), map[string]interface{}{})
}

// applyContexts saves the new active contexts and emits the changes ("chosen" is left to the caller).
func applyContexts(eriaThing producer.ExposedThing, current []string, removed []string, added []string) {
	_stateData.Contexts = current
	saveStateData()
	for _, context := range removed {
		eriaThing.EmitPropertyChange("isActive", false, map[string]interface{}{"context": context})
	}
	for _, context := range added {
		eriaThing.EmitPropertyChange("isActive", true, map[string]interface{}{"context": context})
	}
	eriaThing.EmitPropertyChange("actives", _stateData.Contexts, map[string]interface{}{})
}

// configuredIDs lists the ids of the contexts people choose.
func configuredIDs() []string {
	ids := make([]string, len(config.Contexts))
	for i, context := range config.Contexts {
		ids[i] = context.ID
	}
	return ids
}

// chosenContexts lists the active contexts people choose, in display order.
func chosenContexts() []string {
	chosen := []string{}
	for _, id := range configuredIDs() {
		if slices.Contains(_stateData.Contexts, id) {
			chosen = append(chosen, id)
		}
	}
	return chosen
}

func saveStateData() {
	go publishStates(slices.Clone(_stateData.Contexts))
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

	// Contexts people choose, with their names: a multiple choice for the UIs
	choices := make([]dataSchema.ConstSchema, len(config.Contexts))
	for i, context := range config.Contexts {
		name := context.Name
		if name == "" {
			name = context.ID
		}
		choices[i] = dataSchema.ConstSchema{Const: context.ID, Title: name}
	}
	chosenItems, _ := dataSchema.NewString(dataSchema.StringOneOf(choices...))
	chosenData, _ := dataSchema.NewArray(dataSchema.ArrayItems(chosenItems))
	chosenProperty := interaction.NewProperty(
		"chosen",
		"Contexts",
		"Contexts people choose that are currently active",
		chosenData,
	)
	td.AddProperty(chosenProperty)

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

	eriaThing.SetPropertyReadHandler("chosen", func(t producer.ExposedThing, name string, parameters map[string]interface{}) (interface{}, error) {
		return chosenContexts(), nil
	})

	// go-wot emits the "chosen" change itself, after this handler
	eriaThing.SetPropertyWriteHandler("chosen", func(t producer.ExposedThing, name string, value interface{}, parameters map[string]interface{}) error {
		configured := configuredIDs()
		chosen := []string{}
		for _, v := range value.([]interface{}) {
			id, ok := v.(string)
			if !ok || !slices.Contains(configured, id) {
				return fmt.Errorf("unknown context %v", v)
			}
			chosen = append(chosen, id)
		}
		current, removed, added := lib.SetChosen(_stateData.Contexts, configured, chosen)
		applyContexts(t, current, removed, added)
		return nil
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
			if slices.Contains(configuredIDs(), context) {
				eriaThing.EmitPropertyChange("chosen", chosenContexts(), map[string]interface{}{})
			}
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
			if slices.Contains(configuredIDs(), context) {
				eriaThing.EmitPropertyChange("chosen", chosenContexts(), map[string]interface{}{})
			}
			return true, nil
		}
		return false, nil // Nothing was done
	})
}
