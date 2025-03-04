// Copyright 2022 Juan Pablo Tosso and the OWASP Coraza contributors
// SPDX-License-Identifier: Apache-2.0

package corazawaf

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/corazawaf/coraza/v3/internal/corazarules"
	"github.com/corazawaf/coraza/v3/macro"
	"github.com/corazawaf/coraza/v3/rules"
	"github.com/corazawaf/coraza/v3/types"
	"github.com/corazawaf/coraza/v3/types/variables"
)

// ruleActionParams is used as a wrapper to store the action name
// and parameters, basically for logging purposes.
type ruleActionParams struct {
	// The name of the action, used for logging
	Name string

	// Parameters used by the action
	Param string

	// The action to be executed
	Function rules.Action
}

// Operator is a container for an operator,
type ruleOperatorParams struct {
	// Operator to be used
	Operator rules.Operator

	// Function name (ex @rx)
	Function string
	// Data to initialize the operator
	Data string
	// If true, rule will match if op.Evaluate returns false
	Negation bool
}

type ruleVariableException struct {
	// The string key for the variable that is going to be requested
	// If KeyRx is not nil, KeyStr is ignored
	KeyStr string

	// The key for the variable that is going to be requested
	// If nil, KeyStr is going to be used
	KeyRx *regexp.Regexp
}

// RuleVariable is compiled during runtime by transactions
// to get values from the transaction's variables
// It supports xml, regex, exceptions and many more features
type ruleVariableParams struct {
	// We store the name for performance
	Name string

	// If true, the count of results will be returned
	Count bool

	// The VARIABLE that will be requested
	Variable variables.RuleVariable

	// The key for the variable that is going to be requested
	// If nil, KeyStr is going to be used
	KeyRx *regexp.Regexp

	// The string key for the variable that is going to be requested
	// If KeyRx is not nil, KeyStr is ignored
	KeyStr string

	// A slice of key exceptions
	Exceptions []ruleVariableException
}

type ruleTransformationParams struct {
	// The transformation to be used, used for logging
	Name string

	// The transformation function to be used
	Function rules.Transformation
}

// Rule is used to test a Transaction against certain operators
// and execute actions
type Rule struct {
	corazarules.RuleMetadata
	// Contains a list of variables that will be compiled
	// by a transaction
	variables []ruleVariableParams

	// Contains a pointer to the operator struct used
	// SecActions and SecMark can have nil Operators
	operator *ruleOperatorParams

	// List of transformations to be evaluated
	// In the future, transformations might be run by the
	// action itself, not sure yet
	transformations []ruleTransformationParams

	// Slice of initialized actions to be evaluated during
	// the rule evaluation process
	actions []ruleActionParams

	// Contains the Id of the parent rule if you are inside
	// a chain. Otherwise, it will be 0
	ParentID_ int

	// Capture is used by the transaction to tell the operator
	// to capture variables on TX:0-9
	Capture bool

	// Contains the child rule to chain, nil if there are no chains
	Chain *Rule

	// DisruptiveStatus is the status that will be set to interruptions
	// by disruptive rules
	DisruptiveStatus int

	// Message text to be macro expanded and logged
	// In future versions we might use a special type of string that
	// supports cached macro expansions. For performance
	Msg macro.Macro

	// Rule logdata
	LogData macro.Macro

	// If true, triggering this rule write to the error log
	Log bool

	// If true, triggering this rule write to the audit log
	Audit bool

	// If true, the transformations will be multi matched
	MultiMatch bool

	// Used for error logging
	Disruptive bool

	HasChain bool
}

func (r *Rule) ParentID() int {
	return r.ParentID_
}

func (r *Rule) Status() int {
	return r.DisruptiveStatus
}

// Evaluate will evaluate the current rule for the indicated transaction
// If the operator matches, actions will be evaluated, and it will return
// the matched variables, keys and values (MatchData)
func (r *Rule) Evaluate(tx rules.TransactionState) []types.MatchData {
	return r.doEvaluate(tx.(*Transaction))
}

func (r *Rule) doEvaluate(tx *Transaction) []types.MatchData {
	if r.Capture {
		tx.Capture = true
	}
	rid := r.ID_
	if rid == 0 {
		rid = r.ParentID_
	}

	var matchedValues []types.MatchData
	// we log if we are the parent rule
	tx.WAF.Logger.Debug("[%s] [%d] Evaluating rule %d", tx.id, rid, r.ID_)
	defer tx.WAF.Logger.Debug("[%s] [%d] Finish evaluating rule %d", tx.id, rid, r.ID_)
	ruleCol := tx.variables.rule
	ruleCol.SetIndex("id", 0, strconv.Itoa(rid))
	if r.Msg != nil {
		ruleCol.SetIndex("msg", 0, r.Msg.String())
	}
	ruleCol.SetIndex("rev", 0, r.Rev_)
	if r.LogData != nil {
		ruleCol.SetIndex("logdata", 0, r.LogData.String())
	}
	ruleCol.SetIndex("severity", 0, r.Severity_.String())
	// SecMark and SecAction uses nil operator
	if r.operator == nil {
		tx.WAF.Logger.Debug("[%s] [%d] Forcing rule %d to match", tx.id, rid, r.ID_)
		md := &corazarules.MatchData{}
		matchedValues = append(matchedValues, md)
		r.matchVariable(tx, md)
	} else {
		ecol := tx.ruleRemoveTargetByID[r.ID_]
		for _, v := range r.variables {
			var values []types.MatchData
			for _, c := range ecol {
				if c.Variable == v.Variable {
					// TODO shall we check the pointer?
					v.Exceptions = append(v.Exceptions, ruleVariableException{c.KeyStr, nil})
				}
			}

			values = tx.GetField(v)
			tx.WAF.Logger.Debug("[%s] [%d] Expanding %d arguments for rule %d", tx.id, rid, len(values), r.ID_)
			for _, arg := range values {
				var args []string
				tx.WAF.Logger.Debug("[%s] [%d] Transforming argument %q for rule %d", tx.id, rid, arg.Value(), r.ID_)
				var errs []error
				if r.MultiMatch {
					// TODO in the future, we don't need to run every transformation
					// We could try for each until found
					args, errs = r.executeTransformationsMultimatch(arg.Value())
				} else {
					ars, es := r.executeTransformations(arg.Value())
					args = []string{ars}
					errs = es
				}
				if len(errs) > 0 {
					tx.WAF.Logger.Debug("[%s] [%d] Error transforming argument %q for rule %d: %v", tx.id, rid, arg.Value(), r.ID_, errs)
				}
				tx.WAF.Logger.Debug("[%s] [%d] Arguments transformed for rule %d: %v", tx.id, rid, r.ID_, args)

				// args represents the transformed variables
				for _, carg := range args {
					match := r.executeOperator(carg, tx)
					if match {
						mr := &corazarules.MatchData{
							VariableName_: v.Variable.Name(),
							Variable_:     arg.Variable(),
							Key_:          arg.Key(),
							Value_:        carg,
						}
						// Set the txn variables for expansions before usage
						r.matchVariable(tx, mr)

						if r.Msg != nil {
							mr.Message_ = r.Msg.Expand(tx)
						}
						if r.LogData != nil {
							mr.Data_ = r.LogData.Expand(tx)
						}
						matchedValues = append(matchedValues, mr)

						tx.WAF.Logger.Debug("[%s] [%d] Evaluating operator \"%s %s\" against %q: MATCH",
							tx.id,
							rid,
							r.operator.Function,
							r.operator.Data,
							carg,
						)
					} else {
						tx.WAF.Logger.Debug("[%s] [%d] Evaluating operator \"%s %s\" against %q: NO MATCH",
							tx.id,
							rid,
							r.operator.Function,
							r.operator.Data,
							carg,
						)
					}
				}
			}
		}
	}

	if len(matchedValues) == 0 {
		return matchedValues
	}

	// disruptive actions are only evaluated by parent rules
	if r.ParentID_ == 0 {
		// we only run the chains for the parent rule
		for nr := r.Chain; nr != nil; {
			tx.WAF.Logger.Debug("[%s] [%d] Evaluating rule chain for %d", tx.id, rid, r.ID_)
			matchedChainValues := nr.Evaluate(tx)
			if len(matchedChainValues) == 0 {
				return matchedChainValues
			}
			matchedValues = append(matchedValues, matchedChainValues...)
			nr = nr.Chain
		}
		// we need to add disruptive actions in the end, otherwise they would be triggered without their chains.
		if tx.RuleEngine != types.RuleEngineDetectionOnly {
			tx.WAF.Logger.Debug("[%s] [%d] Disrupting transaction by rule %d", tx.id, rid, r.ID_)
			for _, a := range r.actions {
				if a.Function.Type() == rules.ActionTypeDisruptive || a.Function.Type() == rules.ActionTypeFlow {
					tx.WAF.Logger.Debug("[%s] [%d] Evaluating action %s for rule %d", tx.id, rid, a.Name, r.ID_)
					a.Function.Evaluate(r, tx)
				}
			}

		}
		if r.ID_ != 0 {
			// we avoid matching chains and secmarkers
			tx.MatchRule(r, matchedValues)
		}
	}
	return matchedValues
}

func (r *Rule) matchVariable(tx *Transaction, m *corazarules.MatchData) {
	rid := r.ID_
	if rid == 0 {
		rid = r.ParentID_
	}
	if !m.IsNil() {
		tx.WAF.Logger.Debug("[%s] [%d] Matching rule %d %s:%s", tx.id, rid, r.ID_, m.VariableName(), m.Key())
	}
	// we must match the vars before running the chains

	// We run non-disruptive actions even if there is no chain match
	tx.matchVariable(m)
	for _, a := range r.actions {
		if a.Function.Type() == rules.ActionTypeNondisruptive {
			tx.WAF.Logger.Debug("[%s] [%d] Evaluating action %s for rule %d", tx.id, rid, a.Name, r.ID_)
			a.Function.Evaluate(r, tx)
		}
	}
}

// AddAction adds an action to the rule
func (r *Rule) AddAction(name string, action rules.Action) error {
	// TODO add more logic, like one persistent action per rule etc
	r.actions = append(r.actions, ruleActionParams{
		Name:     name,
		Function: action,
	})
	return nil
}

// AddVariable adds a variable to the rule
// The key can be a regexp.Regexp, a string or nil, in case of regexp
// it will be used to match the variable, in case of string it will
// be a fixed match, in case of nil it will match everything
func (r *Rule) AddVariable(v variables.RuleVariable, key string, iscount bool) error {
	var re *regexp.Regexp
	if len(key) > 2 && key[0] == '/' && key[len(key)-1] == '/' {
		key = key[1 : len(key)-1]
		re = regexp.MustCompile(key)
	}

	r.variables = append(r.variables, ruleVariableParams{
		Name:       v.Name(),
		Count:      iscount,
		Variable:   v,
		KeyStr:     strings.ToLower(key),
		KeyRx:      re,
		Exceptions: []ruleVariableException{},
	})
	return nil
}

// AddVariableNegation adds an exception to a variable
// It passes through if the variable is not used
// It returns an error if the selector is empty,
// or applied on an undefined rule
// for example:
// OK: SecRule ARGS|!ARGS:id "..."
// OK: SecRule !ARGS:id "..."
// ERROR: SecRule !ARGS: "..."
func (r *Rule) AddVariableNegation(v variables.RuleVariable, key string) error {
	var re *regexp.Regexp
	if len(key) > 2 && key[0] == '/' && key[len(key)-1] == '/' {
		key = key[1 : len(key)-1]
		re = regexp.MustCompile(key)
	}
	// Prevent sigsev
	if r == nil {
		return fmt.Errorf("cannot create a variable exception for an undefined rule")
	}
	for i, rv := range r.variables {
		if rv.Variable == v {
			rv.Exceptions = append(rv.Exceptions, ruleVariableException{strings.ToLower(key), re})
			r.variables[i] = rv
		}
	}
	return nil
}

// AddTransformation adds a transformation to the rule
// it fails if the transformation cannot be found
func (r *Rule) AddTransformation(name string, t rules.Transformation) error {
	if t == nil || name == "" {
		return fmt.Errorf("invalid transformation %q not found", name)
	}
	r.transformations = append(r.transformations, ruleTransformationParams{name, t})
	return nil
}

// ClearTransformations clears all the transformations
// it is mostly used by the "none" transformation
func (r *Rule) ClearTransformations() {
	r.transformations = []ruleTransformationParams{}
}

// SetOperator sets the operator of the rule
// There can be only one operator per rule
// functionName and params are used for logging
func (r *Rule) SetOperator(operator rules.Operator, functionName string, params string) {
	r.operator = &ruleOperatorParams{
		Operator: operator,
		Function: functionName,
		Data:     params,
		Negation: len(functionName) > 0 && functionName[0] == '!',
	}
}

func (r *Rule) executeOperator(data string, tx *Transaction) (result bool) {
	result = r.operator.Operator.Evaluate(tx, data)
	if r.operator.Negation {
		result = !result
	}
	return
}

func (r *Rule) executeTransformationsMultimatch(value string) ([]string, []error) {
	res := []string{value}
	var errs []error
	var err error
	for _, t := range r.transformations {
		value, err = t.Function(value)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		res = append(res, value)
	}
	return res, errs
}

func (r *Rule) executeTransformations(value string) (string, []error) {
	var errs []error
	for _, t := range r.transformations {
		v, err := t.Function(value)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		value = v
	}
	return value, errs
}

// NewRule returns a new initialized rule
func NewRule() *Rule {
	return &Rule{
		RuleMetadata: corazarules.RuleMetadata{
			Phase_: 2,
			Tags_:  []string{},
		},
	}
}
