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
import {
    BOOKMARK_STORAGE_PREFIX,
    BookmarkKind,
    BookmarkService,
    BookmarkTarget,
    MAX_BOOKMARKS,
    bookmarkLink,
} from './bookmark.service';
import { SessionService } from './session.service';

describe('BookmarkService', () => {
    let username: string;
    let service: BookmarkService;
    const ids: Record<string, number> = { alice: 1, bob: 2 };
    const session = {
        getCurrentUser: () =>
            username ? { username, user_id: ids[username] } : null,
    } as unknown as SessionService;

    const project: BookmarkTarget = {
        kind: BookmarkKind.PROJECT,
        projectId: 1,
        projectName: 'library',
        defaultLabel: 'library',
    };
    const tag: BookmarkTarget = {
        kind: BookmarkKind.TAG,
        projectId: 1,
        projectName: 'library',
        repoName: 'team/app',
        tag: 'latest',
        defaultLabel: 'team/app:latest',
    };

    function stored(user: string): any {
        return JSON.parse(
            localStorage.getItem(BOOKMARK_STORAGE_PREFIX + ids[user])
        );
    }

    beforeEach(() => {
        localStorage.removeItem(BOOKMARK_STORAGE_PREFIX + 1);
        localStorage.removeItem(BOOKMARK_STORAGE_PREFIX + 2);
        username = 'alice';
        service = new BookmarkService(session);
    });

    afterEach(() => service.ngOnDestroy());

    it('persists added bookmarks per user', () => {
        const b = service.add(project);
        expect(b.label).toBe('library');
        expect(stored('alice').v).toBe(1);
        expect(stored('alice').items.length).toBe(1);

        username = 'bob';
        expect(service.load()).toBeTrue();
        expect(service.bookmarks).toEqual([]);
        expect(stored('bob')).toBeNull();
    });

    it('keeps a bookmark another tab wrote before its storage event', () => {
        service.add(project);
        const other = new BookmarkService(session);
        other.add(tag);
        other.ngOnDestroy();
        service.rename(service.bookmarks[0].id, 'mine');
        expect(stored('alice').items.map(b => b.label)).toEqual([
            'mine',
            'team/app:latest',
        ]);
    });

    it('returns null instead of throwing when storage is blocked', () => {
        service.load();
        spyOn(localStorage, 'getItem').and.throwError('SecurityError');
        spyOn(localStorage, 'setItem').and.throwError('SecurityError');
        expect(service.add(project)).toBeNull();
    });

    it('keeps entries missing their kind fields out of the list but in storage', () => {
        const broken = {
            ...tag,
            tag: undefined,
            id: 'x',
            label: 'x',
            createdAt: 1,
        };
        localStorage.setItem(
            BOOKMARK_STORAGE_PREFIX + 1,
            JSON.stringify({ v: 1, items: [broken] })
        );
        service.load();
        expect(service.bookmarks).toEqual([]);
        service.add(project);
        expect(stored('alice').items.length).toBe(2);
    });

    it('does not duplicate an existing target', () => {
        const first = service.add(tag);
        const second = service.add({ ...tag, defaultLabel: 'other' });
        expect(second.id).toBe(first.id);
        expect(service.bookmarks.length).toBe(1);
        expect(service.find(tag).id).toBe(first.id);
    });

    it('distinguishes targets that differ only by query', () => {
        const filter: BookmarkTarget = {
            kind: BookmarkKind.ARTIFACT_FILTER,
            projectId: 1,
            projectName: 'library',
            repoName: 'team/app',
            query: 'tags=~v1',
            defaultLabel: 'v1',
        };
        service.add(filter);
        service.add({ ...filter, query: 'tags=~v2' });
        expect(service.bookmarks.length).toBe(2);
    });

    it('refuses to add without a signed-in user', () => {
        username = null;
        expect(service.add(project)).toBeNull();
    });

    it('stops at the bookmark limit', () => {
        for (let i = 0; i < MAX_BOOKMARKS; i++) {
            expect(service.add({ ...project, projectId: i })).not.toBeNull();
        }
        expect(service.add({ ...project, projectId: 9999 })).toBeNull();
        expect(service.bookmarks.length).toBe(MAX_BOOKMARKS);
    });

    it('renames, and falls back to the default label when blank', () => {
        const b = service.add(project);
        service.rename(b.id, '  Main project  ');
        expect(service.bookmarks[0].label).toBe('Main project');
        service.rename(b.id, '   ');
        expect(service.bookmarks[0].label).toBe('library');
        expect(stored('alice').items[0].label).toBe('library');
    });

    it('removes a bookmark', () => {
        const a = service.add(project);
        service.add(tag);
        service.remove(a.id);
        expect(service.bookmarks.map(b => b.kind)).toEqual([BookmarkKind.TAG]);
    });

    it('reorders, ignoring orders that do not match the list', () => {
        const a = service.add(project);
        const b = service.add(tag);
        const c = service.add({ ...project, projectId: 2 });
        service.reorder([c.id, a.id, b.id]);
        expect(service.bookmarks.map(x => x.id)).toEqual([c.id, a.id, b.id]);
        service.reorder([a.id, b.id]);
        service.reorder([a.id, a.id, b.id]);
        service.reorder([a.id, b.id, 'nope']);
        expect(service.bookmarks.map(x => x.id)).toEqual([c.id, a.id, b.id]);
    });

    it('keys storage by user id, so a renamed user keeps their bookmarks', () => {
        service.add(project);
        expect(
            localStorage.getItem(BOOKMARK_STORAGE_PREFIX + 1)
        ).not.toBeNull();
        expect(
            localStorage.getItem(BOOKMARK_STORAGE_PREFIX + 'alice')
        ).toBeNull();
    });

    it('writes back entries it does not understand', () => {
        const future = { id: 'f', kind: 'future-kind', projectId: 1 };
        localStorage.setItem(
            BOOKMARK_STORAGE_PREFIX + 2,
            JSON.stringify({ v: 1, items: [future] })
        );
        username = 'bob';
        service.load();
        expect(service.bookmarks).toEqual([]);
        service.add(project);
        expect(stored('bob').items).toEqual([
            jasmine.objectContaining({ kind: BookmarkKind.PROJECT }),
            future,
        ]);
    });

    it('never overwrites a store written by a newer version', () => {
        const raw = JSON.stringify({ v: 2, items: [] });
        localStorage.setItem(BOOKMARK_STORAGE_PREFIX + 2, raw);
        username = 'bob';
        service.load();
        expect(service.add(project)).toBeNull();
        expect(localStorage.getItem(BOOKMARK_STORAGE_PREFIX + 2)).toBe(raw);
    });

    it('ignores corrupt or foreign storage content', () => {
        localStorage.setItem(BOOKMARK_STORAGE_PREFIX + 2, '{not json');
        username = 'bob';
        service.load();
        expect(service.bookmarks).toEqual([]);

        localStorage.setItem(
            BOOKMARK_STORAGE_PREFIX + 2,
            JSON.stringify({
                v: 1,
                items: [{ id: 'x', kind: 'bogus' }, null],
            })
        );
        username = 'alice';
        service.load();
        username = 'bob';
        service.load();
        expect(service.bookmarks).toEqual([]);
    });

    it('picks up changes made in another tab', () => {
        service.load();
        const other = new BookmarkService(session);
        other.add(project);
        window.dispatchEvent(
            new StorageEvent('storage', {
                key: BOOKMARK_STORAGE_PREFIX + 1,
            })
        );
        expect(service.bookmarks.length).toBe(1);
        other.ngOnDestroy();
    });

    it('empties the list when another tab clears localStorage', () => {
        service.add(project);
        localStorage.removeItem(BOOKMARK_STORAGE_PREFIX + 1);
        window.dispatchEvent(new StorageEvent('storage', { key: null }));
        expect(service.bookmarks).toEqual([]);
    });

    it('announces the owner when the signed-in user changes', () => {
        const owners: string[] = [];
        service.owner$.subscribe(o => owners.push(o));
        service.load();
        service.load();
        username = 'bob';
        service.load();
        expect(owners).toEqual([
            null,
            BOOKMARK_STORAGE_PREFIX + 1,
            BOOKMARK_STORAGE_PREFIX + 2,
        ]);
    });

    it('reports when the limit is reached', () => {
        expect(service.isFull).toBeFalse();
        for (let i = 0; i < MAX_BOOKMARKS; i++) {
            service.add({ ...project, projectId: i });
        }
        expect(service.isFull).toBeTrue();
    });

    it('builds router links for every kind', () => {
        expect(bookmarkLink(project).commands).toEqual(['/harbor/projects', 1]);
        expect(
            bookmarkLink({ ...tag, kind: BookmarkKind.REPOSITORY }).commands
        ).toEqual(['/harbor/projects', 1, 'repositories', 'team/app']);
        expect(bookmarkLink(tag).commands).toEqual([
            '/harbor/projects',
            1,
            'repositories',
            'team/app',
            'artifacts-tab',
            'tags',
            'latest',
        ]);
        expect(
            bookmarkLink({
                ...project,
                kind: BookmarkKind.REPOSITORY_FILTER,
                query: 'nginx',
            }).queryParams
        ).toEqual({ name: 'nginx' });
        expect(
            bookmarkLink({
                ...tag,
                kind: BookmarkKind.ARTIFACT_FILTER,
                query: 'labels=(3)',
            })
        ).toEqual({
            commands: [
                '/harbor/projects',
                1,
                'repositories',
                'team/app',
                'artifacts-tab',
            ],
            queryParams: { q: 'labels=(3)' },
        });
    });
});
