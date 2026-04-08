package limiter

import (
	"regexp"

	"github.com/InazumaV/V2bX/api/panel"
)

func (l *Limiter) CheckDomainRule(destination string) (reject bool) {
	// have rule
	for i := range l.DomainRules {
		if l.DomainRules[i].MatchString(destination) {
			reject = true
			break
		}
	}
	return
}

func (l *Limiter) CheckProtocolRule(protocol string) (reject bool) {
	for i := range l.ProtocolRules {
		if l.ProtocolRules[i] == protocol {
			reject = true
			break
		}
	}
	return
}

func (l *Limiter) UpdateRule(rule *panel.Rules) error {
	rules := make([]*regexp.Regexp, 0, len(rule.Regexp))
	for i := range rule.Regexp {
		r, err := regexp.Compile(rule.Regexp[i])
		if err != nil {
			continue
		}
		rules = append(rules, r)
	}
	l.DomainRules = rules
	l.ProtocolRules = rule.Protocol
	return nil
}
