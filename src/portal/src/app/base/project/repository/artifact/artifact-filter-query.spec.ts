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
import { artifactFilterQuery, parseArtifactFilterQuery } from './artifact';

describe('artifact filter query', () => {
    it('round-trips every filter the UI can produce', () => {
        [
            'type=IMAGE',
            'type=CHART',
            'type=CNAB',
            'type=WASM',
            'tags=*',
            'tags=nil',
            'tags=~v1.2',
            'labels=(12)',
        ].forEach(q =>
            expect(artifactFilterQuery(parseArtifactFilterQuery(q))).toBe(q)
        );
    });

    it('parses into filter events', () => {
        expect(parseArtifactFilterQuery('tags=~rc')).toEqual({
            type: 'tags',
            isLabel: false,
            isInputTag: true,
            stringValue: 'rc',
        });
        expect(parseArtifactFilterQuery('labels=(3)')).toEqual({
            type: 'labels',
            isLabel: true,
            label: { id: 3 },
        });
    });

    it('rejects values the filter UI cannot show', () => {
        [
            '',
            null,
            'name=foo',
            'type=FOO',
            'type=~IMAGE',
            'tags=latest',
            'labels=3',
            'labels=~(3)',
            'labels=(abc)',
        ].forEach(q => expect(parseArtifactFilterQuery(q)).toBeNull());
    });

    it('builds no query for empty selections', () => {
        expect(artifactFilterQuery({ type: 'labels', isLabel: true })).toBe('');
        expect(artifactFilterQuery({ type: 'tags', stringValue: '' })).toBe('');
        expect(artifactFilterQuery(null)).toBe('');
    });
});
