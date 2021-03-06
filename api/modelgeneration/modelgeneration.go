// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client provides methods that the Juju client command uses to interact
// with models stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ModelGeneration")
	return &Client{ClientFacade: frontend, facade: backend}
}

// AddGeneration adds a model generation to the config.
func (c *Client) AddGeneration(modelUUID string) error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("AddGeneration", argForModel(modelUUID), &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}

// CancelGeneration cancels a model generation to the config.
func (c *Client) CancelGeneration(modelUUID string) error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("CancelGeneration", argForModel(modelUUID), &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}

// AdvanceGeneration advances a unit and/or applications to the 'next'
// generation. The boolean return indicates whether the generation was
// automatically completed as a result of unit advancement.
func (c *Client) AdvanceGeneration(modelUUID string, entities []string) (bool, error) {
	var result params.AdvanceGenerationResult
	arg := params.AdvanceGenerationArg{Model: argForModel(modelUUID)}
	if len(entities) == 0 {
		return false, errors.Trace(errors.New("No units or applications to advance"))
	}
	for _, entity := range entities {
		switch {
		case names.IsValidApplication(entity):
			arg.Entities = append(arg.Entities,
				params.Entity{Tag: names.NewApplicationTag(entity).String()})
		case names.IsValidUnit(entity):
			arg.Entities = append(arg.Entities,
				params.Entity{Tag: names.NewUnitTag(entity).String()})
		default:
			return false, errors.Trace(errors.New("Must be application or unit"))
		}
	}
	err := c.facade.FacadeCall("AdvanceGeneration", arg, &result)
	if err != nil {
		return false, errors.Trace(err)
	}

	// If there were errors based on the advancing units, return those.
	// Otherwise check the results of auto-completion.
	if err := result.AdvanceResults.Combine(); err != nil {
		return false, errors.Trace(err)
	}
	res := result.CompleteResult
	if res.Error != nil {
		return false, errors.Trace(res.Error)
	}
	return res.Result, nil
}

// HasNextGeneration returns true if the model has a "next" generation that
// has not yet been completed.
func (c *Client) HasNextGeneration(modelUUID string) (bool, error) {
	var result params.BoolResult
	err := c.facade.FacadeCall("HasNextGeneration", argForModel(modelUUID), &result)
	if err != nil {
		return false, errors.Trace(err)
	}
	if result.Error != nil {
		return false, errors.Trace(result.Error)
	}
	return result.Result, nil
}

func argForModel(modelUUID string) params.Entity {
	return params.Entity{Tag: names.NewModelTag(modelUUID).String()}
}
