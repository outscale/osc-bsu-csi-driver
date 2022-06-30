//go:build !providerless
// +build !providerless

/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package osc

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/outscale/osc-sdk-go/v2"
)

// IPRulesSet maps IP strings of strings to OSC IpPermissions
type IPRulesSet map[string]osc.SecurityGroupRule

// NewIPRulesSetFromAWS creates a new IPRulesSet
func NewIPRulesSetFromAWS(items ...*ec2.IpPermission) IPRulesSet {
	s := make(IPRulesSet)

	rules := []osc.SecurityGroupRule{}
	for _, permission := range items {
		securityGroupRule := osc.SecurityGroupRule{}
		if permission.FromPort != nil {
			fromPortValue := int32(*permission.FromPort)
			securityGroupRule.SetFromPortRange(fromPortValue)
		}

		if permission.ToPort != nil {
			toPortValue := int32(*permission.ToPort)
			securityGroupRule.SetToPortRange(toPortValue)
		}

		ipRanges := []string{}
		for _, ipRange := range permission.IpRanges {
			ipRanges = append(ipRanges, *ipRange.CidrIp)
		}
		securityGroupRule.SetIpRanges(ipRanges)

		securityGroupsMembers := []osc.SecurityGroupsMember{}
		for _, group := range permission.UserIdGroupPairs {
			securityGroupsMember := osc.SecurityGroupsMember{
				AccountId:         group.UserId,
				SecurityGroupId:   group.GroupId,
				SecurityGroupName: group.GroupName,
			}

			securityGroupsMembers = append(securityGroupsMembers, securityGroupsMember)
		}
		securityGroupRule.SetSecurityGroupsMembers(securityGroupsMembers)

		// TODO: ServicesIds ?
		rules = append(rules, securityGroupRule)
	}

	s.Insert(rules...)
	return s
}

// NewIPRulesSet creates a new IPRulesSet
func NewIPRulesSet(items ...osc.SecurityGroupRule) IPRulesSet {
	s := make(IPRulesSet)
	s.Insert(items...)
	return s
}

// Ungroup splits permissions out into individual permissions
// EC2 will combine permissions with the same port but different SourceRanges together, for example
// We ungroup them so we can process them
func (s IPRulesSet) Ungroup() IPRulesSet {
	l := []osc.SecurityGroupRule{}
	for _, p := range s.List() {
		if len(p.GetIpRanges()) <= 1 {
			l = append(l, p)
			continue
		}
		for _, ipRange := range p.GetIpRanges() {
			c := osc.SecurityGroupRule{}
			c = p
			c.IpRanges = &[]string{ipRange}
			l = append(l, c)
		}
	}

	l2 := []osc.SecurityGroupRule{}
	for _, p := range l {
		if len(p.GetSecurityGroupsMembers()) <= 1 {
			l2 = append(l2, p)
			continue
		}
		for _, u := range p.GetSecurityGroupsMembers() {
			c := osc.SecurityGroupRule{}
			c = p
			c.SecurityGroupsMembers = &[]osc.SecurityGroupsMember{u}
			l2 = append(l, c)
		}
	}

	l3 := []osc.SecurityGroupRule{}
	for _, p := range l2 {
		if len(p.GetServiceIds()) <= 1 {
			l3 = append(l3, p)
			continue
		}
		for _, v := range p.GetServiceIds() {
			c := osc.SecurityGroupRule{}
			c = p
			c.ServiceIds = &[]string{v}
			l3 = append(l3, c)
		}
	}

	return NewIPRulesSet(l3...)
}

// Insert adds items to the set.
func (s IPRulesSet) Insert(items ...osc.SecurityGroupRule) {
	for _, p := range items {
		k := keyForIPRules(&p)
		s[k] = p
	}
}

// List returns the contents as a slice.  Order is not defined.
func (s IPRulesSet) List() []osc.SecurityGroupRule {
	res := make([]osc.SecurityGroupRule, 0, len(s))
	for _, v := range s {
		res = append(res, v)
	}
	return res
}

// Difference returns a set of objects that are not in s2
// For example:
// s1 = {a1, a2, a3}
// s2 = {a1, a2, a4, a5}
// s1.Difference(s2) = {a3}
// s2.Difference(s1) = {a4, a5}
func (s IPRulesSet) Difference(s2 IPRulesSet) IPRulesSet {
	result := NewIPRulesSet()
	for k, v := range s {
		_, found := s2[k]
		if !found {
			result[k] = v
		}
	}
	return result
}

// Len returns the size of the set.
func (s IPRulesSet) Len() int {
	return len(s)
}

func keyForIPRules(p *osc.SecurityGroupRule) string {
	v, err := json.Marshal(p)
	if err != nil {
		panic(fmt.Sprintf("error building JSON representation of ec2.IpPermission: %v", err))
	}
	return string(v)
}
