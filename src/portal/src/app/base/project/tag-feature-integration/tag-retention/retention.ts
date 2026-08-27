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
export class BaseRetention {
    algorithm: string;
    scope: {
        level: string;
        ref: number;
    };
    cap: number;

    constructor() {
        this.algorithm = 'or';
    }
}
export class Retention extends BaseRetention {
    rules: Array<Rule>;
    trigger: {
        kind: string;
        references: object;
        settings: {
            cron: string;
            next_scheduled_time?: string;
        };
    };
    constructor() {
        super();
        this.rules = [];
        this.trigger = {
            kind: 'Schedule',
            references: {},
            settings: {
                cron: '',
            },
        };
    }
}

// pattern engines a selector can be evaluated with, as advertised by
// GET /retentions/metadatas
export const SELECTOR_KIND_DOUBLESTAR: string = 'doublestar';
export const SELECTOR_KIND_REGEXP: string = 'regexp';

export class BaseRule {
    disabled: boolean;
    template: string;
    id: number;
    priority: number;
    action: string;
    tag_selectors: Array<Selector>;
    scope_selectors: {
        repository: Array<Selector>;
    };

    constructor() {
        this.disabled = false;
        this.action = 'retain';
        this.scope_selectors = {
            repository: [
                {
                    kind: SELECTOR_KIND_DOUBLESTAR,
                    decoration: 'repoMatches',
                    pattern: '**',
                },
            ],
        };
        this.tag_selectors = [
            {
                kind: SELECTOR_KIND_DOUBLESTAR,
                decoration: 'matches',
                pattern: '**',
            },
        ];
    }
}

export class ImmutableRetentionRule extends BaseRule {
    project_id: number;
    params: object;
    constructor(project_id) {
        super();
        this.project_id = project_id;
        this.priority = 0;
        this.params = {};
        this.action = 'immutable';
        this.template = 'immutable_template';
    }
}
// rule for tag-retention
export class Rule extends BaseRule {
    params: object;

    constructor() {
        super();
        this.params = {};
        this.tag_selectors[0].extras = JSON.stringify({ untagged: true });
    }
}

export class Selector {
    kind: string;
    decoration: string;
    pattern: string;
    extras?: string;
}

export class Param {
    type: string;
    unit: string;
    required: boolean;
}

export class Template {
    rule_template: string;
    display_text: string;
    action: 'retain' | 'immutable';
    params: Array<Param>;
}

export class SelectorRuleMetadate {
    display_text: string;
    kind: string;
    decorations: Array<string>;
}

export class RuleMetadate {
    templates: Array<Template>;
    scope_selectors: Array<SelectorRuleMetadate>;
    tag_selectors: Array<SelectorRuleMetadate>;

    constructor() {
        this.templates = [];
        this.scope_selectors = [
            {
                display_text: null,
                kind: null,
                decorations: [],
            },
        ];
        this.tag_selectors = [
            {
                display_text: null,
                kind: null,
                decorations: [],
            },
            {
                display_text: null,
                kind: null,
                decorations: [],
            },
        ];
    }
}

// kinds advertised by the metadata endpoint, in the order the backend lists
// them; the first one is the default for a new rule
export function selectorKinds(
    selectors: Array<SelectorRuleMetadate>
): Array<string> {
    return (selectors || []).map(s => s && s.kind).filter(kind => !!kind);
}

// decorations of the metadata entry for the given kind, falling back to the
// first entry so a backend that advertises a single kind keeps working
export function selectorDecorations(
    selectors: Array<SelectorRuleMetadate>,
    kind: string
): Array<string> {
    const entries = selectors || [];
    const entry = entries.find(s => s && s.kind === kind) || entries[0];
    return (entry && entry.decorations) || [];
}

// The portal wraps a comma separated list in braces because that is doublestar
// syntax for alternation. A regex expresses alternation natively and uses
// braces for quantifiers such as {2,3}, so the pattern is stored verbatim.
export function wrapPattern(pattern: string, kind: string): string {
    if (kind === SELECTOR_KIND_REGEXP) {
        return pattern;
    }
    if (
        pattern.indexOf(',') !== -1 &&
        pattern.indexOf('{') === -1 &&
        pattern.indexOf('}') === -1
    ) {
        return '{' + pattern + '}';
    }
    return pattern;
}

export function unwrapPattern(pattern: string, kind: string): string {
    if (kind === SELECTOR_KIND_REGEXP) {
        return pattern;
    }
    if (/^{\S+}$/.test(pattern)) {
        return pattern.slice(1, pattern.length - 1);
    }
    return pattern;
}

export const RUNNING: string = 'Running';
export const PENDING: string = 'Pending';
export const TIMEOUT: number = 5000;

export enum RetentionAction {
    STOP = 'stop',
}
