// Copyright 2022 Juan Pablo Tosso and the OWASP Coraza contributors
// SPDX-License-Identifier: Apache-2.0

package actions

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/corazawaf/coraza/v3/internal/corazawaf"
	utils "github.com/corazawaf/coraza/v3/internal/strings"
	"github.com/corazawaf/coraza/v3/rules"
	"github.com/corazawaf/coraza/v3/types"
	"github.com/corazawaf/coraza/v3/types/variables"
)

type ctlFunctionType int

const (
	ctlUnknown                  ctlFunctionType = iota
	ctlRuleRemoveTargetByID     ctlFunctionType = iota
	ctlRuleRemoveTargetByTag    ctlFunctionType = iota
	ctlRuleRemoveTargetByMsg    ctlFunctionType = iota
	ctlAuditEngine              ctlFunctionType = iota
	ctlAuditLogParts            ctlFunctionType = iota
	ctlForceRequestBodyVariable ctlFunctionType = iota
	ctlRequestBodyAccess        ctlFunctionType = iota
	ctlRequestBodyLimit         ctlFunctionType = iota
	ctlRuleEngine               ctlFunctionType = iota
	ctlRuleRemoveByID           ctlFunctionType = iota
	ctlRuleRemoveByMsg          ctlFunctionType = iota
	ctlRuleRemoveByTag          ctlFunctionType = iota
	ctlHashEngine               ctlFunctionType = iota
	ctlHashEnforcement          ctlFunctionType = iota
	ctlRequestBodyProcessor     ctlFunctionType = iota
	ctlResponseBodyAccess       ctlFunctionType = iota
	ctlResponseBodyLimit        ctlFunctionType = iota
	ctlDebugLogLevel            ctlFunctionType = iota
)

type ctlFn struct {
	action     ctlFunctionType
	value      string
	collection variables.RuleVariable
	colKey     string
	colRx      *regexp.Regexp
}

func (a *ctlFn) Init(r rules.RuleMetadata, data string) error {
	var err error
	a.action, a.value, a.collection, a.colKey, err = parseCtl(data)
	if len(a.colKey) > 2 && a.colKey[0] == '/' && a.colKey[len(a.colKey)-1] == '/' {
		a.colRx, err = regexp.Compile(a.colKey[1 : len(a.colKey)-1])
		if err != nil {
			return err
		}
	}
	return err
}

// parseOnOff turns a string value into a boolean equivalent on/off into true/false
func parseOnOff(s string) (bool, bool) {
	val := strings.ToLower(s)
	switch val {
	case "on":
		return true, true
	case "off":
		return false, true
	default:
		return false, false
	}
}

func (a *ctlFn) Evaluate(r rules.RuleMetadata, txS rules.TransactionState) {
	// TODO(anuraaga): Confirm this is internal implementation detail
	tx := txS.(*corazawaf.Transaction)
	switch a.action {
	case ctlRuleRemoveTargetByID:
		ran, err := rangeToInts(tx.WAF.Rules.GetRules(), a.value)
		if err != nil {
			tx.WAF.Logger.Error("[ctl:RuleRemoveTargetByID] invalid range: %s", err.Error())
			return
		}
		for _, id := range ran {
			tx.RemoveRuleTargetByID(id, a.collection, a.colKey)
		}
	case ctlRuleRemoveTargetByTag:
		rules := tx.WAF.Rules.GetRules()
		for _, r := range rules {
			if utils.InSlice(a.value, r.Tags_) {
				tx.RemoveRuleTargetByID(r.ID(), a.collection, a.colKey)
			}
		}
	case ctlRuleRemoveTargetByMsg:
		rules := tx.WAF.Rules.GetRules()
		for _, r := range rules {
			if r.Msg != nil && r.Msg.String() == a.value {
				tx.RemoveRuleTargetByID(r.ID(), a.collection, a.colKey)
			}
		}
	case ctlAuditEngine:
		ae, err := types.ParseAuditEngineStatus(a.value)
		if err != nil {
			tx.WAF.Logger.Error("[ctl:AuditEngine] %s", err.Error())
			return
		}
		tx.AuditEngine = ae
	case ctlAuditLogParts:
		// TODO lets switch it to a string
		tx.AuditLogParts = types.AuditLogParts(a.value)
	case ctlForceRequestBodyVariable:
		val, ok := parseOnOff(a.value)
		if !ok {
			tx.WAF.Logger.Error("[ctl:ForceRequestBodyVariable] unknown value %q", a.value)
			return
		}
		tx.ForceRequestBodyVariable = val
		tx.WAF.Logger.Debug("[ctl:ForceRequestBodyVariable] Forcing request body var with CTL to %s", val)
	case ctlRequestBodyAccess:
		val, ok := parseOnOff(a.value)
		if !ok {
			tx.WAF.Logger.Error("[ctl:RequestBodyAccess] unknown value %q", a.value)
			return
		}
		tx.RequestBodyAccess = val
	case ctlRequestBodyLimit:
		limit, err := strconv.ParseInt(a.value, 10, 64)
		if err != nil {
			tx.WAF.Logger.Error("[ctl:RequestBodyLimit] Incorrect integer CTL value %q", a.value)
			return
		}
		tx.RequestBodyLimit = limit
	case ctlRuleEngine:
		re, err := types.ParseRuleEngineStatus(a.value)
		if err != nil {
			tx.WAF.Logger.Error("[ctl:RuleEngine] %s", err.Error())
			return
		}
		tx.RuleEngine = re
	case ctlRuleRemoveByID:
		id, err := strconv.Atoi(a.value)
		if err != nil {
			tx.WAF.Logger.Error("[ctl:RuleRemoveByID] %s", err.Error())
			return
		}
		tx.RemoveRuleByID(id)
	case ctlRuleRemoveByMsg:
		rules := tx.WAF.Rules.GetRules()
		for _, r := range rules {
			if r.Msg != nil && r.Msg.String() == a.value {
				tx.RemoveRuleByID(r.ID_)
			}
		}
	case ctlRuleRemoveByTag:
		rules := tx.WAF.Rules.GetRules()
		for _, r := range rules {
			if utils.InSlice(a.value, r.Tags_) {
				tx.RemoveRuleByID(r.ID_)
			}
		}
	case ctlRequestBodyProcessor:
		tx.Variables().RequestBodyProcessor().Set(strings.ToUpper(a.value))
	case ctlHashEngine:
		// Not supported yet
	case ctlHashEnforcement:
		// Not supported yet
	case ctlDebugLogLevel:
		// lvl, _ := strconv.Atoi(a.Value)
		// TODO
		// We cannot update the log level, it would affect the whole waf instance...
		// tx.WAF.SetLogLevel(lvl)
	}

}

func (a *ctlFn) Type() rules.ActionType {
	return rules.ActionTypeNondisruptive
}

func parseCtl(data string) (ctlFunctionType, string, variables.RuleVariable, string, error) {
	spl1 := strings.SplitN(data, "=", 2)
	if len(spl1) != 2 {
		return ctlUnknown, "", 0, "", errors.New("invalid syntax")
	}
	spl2 := strings.SplitN(spl1[1], ";", 2)
	action := spl1[0]
	value := spl2[0]
	colname := ""
	colkey := ""
	if len(spl2) == 2 {
		spl3 := strings.SplitN(spl2[1], ":", 2)
		if len(spl3) == 2 {
			colname = spl3[0]
			colkey = spl3[1]
		} else {
			colkey = spl3[0]
		}
	}
	collection, _ := variables.Parse(strings.TrimSpace(colname))
	colkey = strings.ToLower(colkey)
	var act ctlFunctionType
	switch action {
	case "auditEngine":
		act = ctlAuditEngine
	case "auditLogParts":
		act = ctlAuditLogParts
	case "forceRequestBodyVariable":
		act = ctlForceRequestBodyVariable
	case "requestBodyAccess":
		act = ctlRequestBodyAccess
	case "requestBodyLimit":
		act = ctlRequestBodyLimit
	case "requestBodyProcessor":
		act = ctlRequestBodyProcessor
	case "responseBodyAccess":
		act = ctlResponseBodyAccess
	case "responseBodyLimit":
		act = ctlResponseBodyLimit
	case "ruleEngine":
		act = ctlRuleEngine
	case "ruleRemoveById":
		act = ctlRuleRemoveByID
	case "ruleRemoveByMsg":
		act = ctlRuleRemoveByMsg
	case "ruleRemoveByTag":
		act = ctlRuleRemoveByTag
	case "ruleRemoveTargetById":
		act = ctlRuleRemoveTargetByID
	case "ruleRemoveTargetByMsg":
		act = ctlRuleRemoveTargetByMsg
	case "ruleRemoveTargetByTag":
		act = ctlRuleRemoveTargetByTag
	case "hashEngine":
		act = ctlHashEngine
	case "hashEnforcement":
		act = ctlHashEnforcement
	default:
		return ctlUnknown, "", 0x00, "", fmt.Errorf("unknown ctl action %q", action)
	}
	return act, value, collection, strings.TrimSpace(colkey), nil
}

func rangeToInts(rules []*corazawaf.Rule, input string) ([]int, error) {
	if len(input) == 0 {
		return nil, errors.New("empty input")
	}

	var (
		ids        []int
		start, end int
		err        error
	)

	spl := strings.SplitN(input, "-", 2)
	if len(spl) == 2 {
		start, err = strconv.Atoi(spl[0])
		if err != nil {
			return nil, err
		}
		end, err = strconv.Atoi(spl[1])
		if err != nil {
			return nil, err
		}
	} else {
		id, err := strconv.Atoi(input)
		if err != nil {
			return nil, err
		}
		start, end = id, id
	}

	for _, r := range rules {
		if r.ID_ >= start && r.ID_ <= end {
			ids = append(ids, r.ID_)
		}
	}
	return ids, nil
}

func ctl() rules.Action {
	return &ctlFn{}
}

var (
	_ rules.Action      = &ctlFn{}
	_ ruleActionWrapper = ctl
)
