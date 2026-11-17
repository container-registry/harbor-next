// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package summary turns an audit log entry into one plain sentence, e.g.
// "alice pushed artifact library/nginx:1.25 (sha256:7f3a...)".
package summary

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/goharbor/harbor/src/pkg/auditext/model"
)

// MaxLength is the width of the audit_log_ext.op_desc column.
const MaxLength = 1024

// configurationType entries list their changes after a colon, as written.
const configurationType = "configuration"

type verb struct {
	base string
	past string
}

var verbs = map[string]verb{
	"create": {"create", "created"},
	"update": {"update", "updated"},
	"delete": {"delete", "deleted"},
	"pull":   {"pull", "pulled"},
	"push":   {"push", "pushed"},
	"login":  {"log in", "logged in"},
	"logout": {"log out", "logged out"},
}

// Describe returns the summary sentence for the entry. The resolver's own
// description is kept as a trailing detail when it says more than the
// operation, resource type and resource already do.
func Describe(a *model.AuditLogExt) string {
	if a == nil {
		return ""
	}
	actor := a.Username
	if actor == "" {
		actor = "unknown"
	}
	op := operationWord(a)
	v, ok := verbs[op]
	if !ok {
		v = verb{op, op}
	}

	var b strings.Builder
	b.WriteString(actor)
	if a.IsSuccessful {
		b.WriteString(" " + v.past)
	} else {
		b.WriteString(" failed to " + v.base)
	}
	if !selfTarget(a) {
		if a.ResourceType != "" {
			b.WriteString(" " + a.ResourceType)
		}
		if a.Resource != "" && a.Resource != a.ResourceType {
			b.WriteString(" " + a.Resource)
		}
	}
	if d := detail(a, op); d != "" && a.ResourceType == configurationType {
		b.WriteString(": " + d)
	} else if d != "" {
		b.WriteString(" (" + d + ")")
	}
	return truncate(b.String(), MaxLength)
}

// operationWord names an artifact create a push, which is what the client did.
func operationWord(a *model.AuditLogExt) string {
	op := strings.ToLower(a.Operation)
	if op == "create" && a.ResourceType == "artifact" {
		return "push"
	}
	return op
}

// selfTarget reports entries whose resource is the actor itself (login, logout).
func selfTarget(a *model.AuditLogExt) bool {
	op := strings.ToLower(a.Operation)
	return (op == "login" || op == "logout") && (a.Resource == "" || a.Resource == a.Username)
}

// detail strips the "<op> <type>[ with name][:] <resource>" boilerplate the
// resolvers write and returns what is left.
func detail(a *model.AuditLogExt, op string) string {
	desc := strings.TrimSpace(a.OperationDescription)
	if desc == "" {
		return ""
	}
	lower := strings.ToLower(desc)
	for _, prefix := range boilerplate(a, op) {
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		rest := desc[len(prefix):]
		if rest != "" && !strings.HasPrefix(rest, " ") && !strings.HasPrefix(rest, ":") && !strings.HasPrefix(rest, ",") {
			continue
		}
		rest = strings.TrimPrefix(rest, " with name")
		rest = strings.TrimLeft(rest, ": ")
		if rest == "" || a.ResourceType == configurationType {
			return rest
		}
		// the resource as a whole, which may contain spaces
		if tail, ok := strings.CutPrefix(rest, a.Resource); ok && a.Resource != "" &&
			(tail == "" || strings.ContainsRune(" ,(", rune(tail[0]))) {
			return trimDetail(tail)
		}
		token, tail, _ := strings.Cut(rest, " ")
		token = strings.TrimSuffix(token, ",")
		parts := make([]string, 0, 2)
		if t := resourceDetail(a, token); t != "" {
			parts = append(parts, t)
		}
		if tail = trimDetail(tail); tail != "" {
			parts = append(parts, tail)
		}
		return strings.Join(parts, ", ")
	}
	if desc == a.Resource {
		return ""
	}
	return desc
}

// trimDetail drops the separator and parentheses around a trailing detail.
func trimDetail(tail string) string {
	tail = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(tail), ","))
	return strings.TrimSuffix(strings.TrimPrefix(tail, "("), ")")
}

func boilerplate(a *model.AuditLogExt, op string) []string {
	rawOp := strings.ToLower(a.Operation)
	rt := strings.ToLower(a.ResourceType)
	prefixes := []string{}
	for _, o := range []string{op, rawOp} {
		if rt != "" {
			prefixes = append(prefixes, o+" "+rt)
		}
		prefixes = append(prefixes, o)
	}
	return prefixes
}

// resourceDetail drops the token when it only repeats the resource, and keeps
// the digest when the resource names the artifact by tag.
func resourceDetail(a *model.AuditLogExt, token string) string {
	if token == "" || token == a.Resource {
		return ""
	}
	if repo, dgst, ok := strings.Cut(token, "@"); ok {
		if a.Resource == token || strings.HasPrefix(a.Resource, repo+"@") {
			return ""
		}
		if strings.HasPrefix(a.Resource, repo+":") {
			return dgst
		}
	}
	if strings.HasPrefix(token, "(") {
		return strings.TrimSuffix(strings.TrimPrefix(token, "("), ")")
	}
	return token
}

// ReplaceActor replaces the actor heading a summary, as the GDPR masking of a
// deleted user does to the rows already stored.
func ReplaceActor(desc, actor, replaceWith string) string {
	rest, ok := strings.CutPrefix(desc, actor+" ")
	if !ok || actor == "" {
		return desc
	}
	return truncate(replaceWith+" "+rest, MaxLength)
}

func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return fmt.Sprintf("%s...", string(runes[:maxRunes-3]))
}
