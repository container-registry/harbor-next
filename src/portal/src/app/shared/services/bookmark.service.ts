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
import { Injectable, OnDestroy } from '@angular/core';
import { ActivatedRoute, Params, Router } from '@angular/router';
import { BehaviorSubject, Observable } from 'rxjs';
import { distinctUntilChanged, map } from 'rxjs/operators';
import { SessionService } from './session.service';

export enum BookmarkKind {
    PROJECT = 'project',
    REPOSITORY = 'repository',
    TAG = 'tag',
    REPOSITORY_FILTER = 'repository-filter',
    ARTIFACT_FILTER = 'artifact-filter',
}

export interface BookmarkTarget {
    kind: BookmarkKind;
    projectId: number;
    projectName: string;
    // repository name without the project prefix, as used in routes
    repoName?: string;
    tag?: string;
    // REPOSITORY_FILTER: repository name fragment; ARTIFACT_FILTER: API q expression
    query?: string;
    defaultLabel: string;
}

export interface Bookmark extends BookmarkTarget {
    id: string;
    label: string;
    createdAt: number;
}

export interface BookmarkLink {
    commands: any[];
    queryParams?: Params;
}

export const BOOKMARK_STORAGE_PREFIX = 'harbor-bookmarks:';
export const BOOKMARK_STORAGE_VERSION = 1;
export const MAX_BOOKMARKS = 100;
export const MAX_BOOKMARK_LABEL_LENGTH = 100;
export const REPOSITORY_FILTER_PARAM = 'name';
export const ARTIFACT_FILTER_PARAM = 'q';

const KINDS: string[] = Object.values(BookmarkKind);

export function bookmarkKey(t: BookmarkTarget): string {
    return JSON.stringify([
        t.kind,
        +t.projectId,
        t.repoName || '',
        t.tag || '',
        t.query || '',
    ]);
}

export function bookmarkLink(b: BookmarkTarget): BookmarkLink {
    const project = ['/harbor/projects', b.projectId];
    switch (b.kind) {
        case BookmarkKind.PROJECT:
            return { commands: project };
        case BookmarkKind.REPOSITORY:
            return { commands: [...project, 'repositories', b.repoName] };
        case BookmarkKind.TAG:
            return {
                commands: [
                    ...project,
                    'repositories',
                    b.repoName,
                    'artifacts-tab',
                    'tags',
                    b.tag,
                ],
            };
        case BookmarkKind.REPOSITORY_FILTER:
            return {
                commands: [...project, 'repositories'],
                queryParams: { [REPOSITORY_FILTER_PARAM]: b.query },
            };
        case BookmarkKind.ARTIFACT_FILTER:
            return {
                commands: [
                    ...project,
                    'repositories',
                    b.repoName,
                    'artifacts-tab',
                ],
                queryParams: { [ARTIFACT_FILTER_PARAM]: b.query },
            };
    }
}

// A query-only navigation reuses the list component, so typing keeps input focus,
// while the router still knows the URL for highlighting and post-login redirects.
export function replaceQueryParam(
    router: Router,
    route: ActivatedRoute,
    key: string,
    value: string | null
): void {
    if ((route.snapshot.queryParamMap.get(key) || null) === (value || null)) {
        return;
    }
    router.navigate([], {
        relativeTo: route,
        queryParams: { [key]: value || null },
        queryParamsHandling: 'merge',
        replaceUrl: true,
    });
}

function isValidBookmark(b: any): b is Bookmark {
    return (
        !!b &&
        typeof b.id === 'string' &&
        KINDS.indexOf(b.kind) !== -1 &&
        Number.isFinite(b.projectId) &&
        typeof b.projectName === 'string' &&
        typeof b.label === 'string' &&
        typeof b.defaultLabel === 'string'
    );
}

function newId(): string {
    // crypto.randomUUID is unavailable on plain-http deployments
    return (
        Date.now().toString(36) + Math.random().toString(36).substring(2, 10)
    );
}

@Injectable({
    providedIn: 'root',
})
export class BookmarkService implements OnDestroy {
    private readonly items = new BehaviorSubject<Bookmark[]>([]);
    readonly bookmarks$: Observable<Bookmark[]> = this.items.asObservable();
    private readonly owner = new BehaviorSubject<string | null>(null);
    // Emits the storage key of the signed-in user whenever it changes.
    readonly owner$: Observable<string | null> = this.owner.asObservable();
    private storageKey: string = null;
    // Entries this version cannot read are written back untouched instead of dropped.
    private foreign: unknown[] = [];
    // A store written by a newer release is never overwritten by this one.
    private readOnly = false;
    // The stored text this tab last read or wrote.
    private lastRaw: string | null = null;

    // A null key means another tab called localStorage.clear().
    private readonly onStorage = (e: StorageEvent) => {
        if (this.storageKey && (e.key === null || e.key === this.storageKey)) {
            this.items.next(this.read());
        }
    };

    constructor(private session: SessionService) {
        window.addEventListener('storage', this.onStorage);
    }

    ngOnDestroy(): void {
        window.removeEventListener('storage', this.onStorage);
    }

    get bookmarks(): Bookmark[] {
        return this.items.value;
    }

    get isFull(): boolean {
        return this.items.value.length >= MAX_BOOKMARKS;
    }

    load(): boolean {
        // The user id, unlike the name, survives a rename and is never reused.
        const userId = this.session.getCurrentUser()?.user_id;
        const key = userId ? BOOKMARK_STORAGE_PREFIX + userId : null;
        if (key !== this.storageKey) {
            this.storageKey = key;
            const items = key ? this.read() : [];
            // Owner first, so listeners reset before the new user's list arrives.
            this.owner.next(key);
            this.items.next(items);
        }
        return !!key;
    }

    find(target: BookmarkTarget): Bookmark | undefined {
        const key = bookmarkKey(target);
        return this.items.value.find(b => bookmarkKey(b) === key);
    }

    watch(target: BookmarkTarget): Observable<Bookmark | undefined> {
        const key = bookmarkKey(target);
        return this.items.pipe(
            map(items => items.find(b => bookmarkKey(b) === key)),
            distinctUntilChanged()
        );
    }

    add(target: BookmarkTarget): Bookmark | null {
        if (!this.load()) {
            return null;
        }
        this.sync();
        const existing = this.find(target);
        if (existing) {
            return existing;
        }
        if (this.isFull) {
            return null;
        }
        const bookmark: Bookmark = {
            kind: target.kind,
            projectId: +target.projectId,
            projectName: target.projectName,
            repoName: target.repoName,
            tag: target.tag,
            query: target.query,
            defaultLabel: target.defaultLabel,
            label: target.defaultLabel,
            id: newId(),
            createdAt: Date.now(),
        };
        return this.write([...this.items.value, bookmark]) ? bookmark : null;
    }

    remove(id: string): void {
        this.sync();
        this.write(this.items.value.filter(b => b.id !== id));
    }

    rename(id: string, label: string): void {
        const trimmed = (label || '')
            .trim()
            .substring(0, MAX_BOOKMARK_LABEL_LENGTH);
        this.sync();
        this.write(
            this.items.value.map(b =>
                b.id === id ? { ...b, label: trimmed || b.defaultLabel } : b
            )
        );
    }

    // Applies a new order; ids that do not match the current list are ignored.
    reorder(ids: string[]): void {
        this.sync();
        const byId = new Map(this.items.value.map(b => [b.id, b]));
        const items = ids.map(id => byId.get(id)).filter(b => !!b);
        if (
            items.length === byId.size &&
            new Set(ids).size === ids.length &&
            items.some((b, i) => b !== this.items.value[i])
        ) {
            this.write(items);
        }
    }

    // Another tab may have written after our last storage event; edit its list.
    private sync(): void {
        if (!this.storageKey) {
            return;
        }
        let raw: string | null;
        try {
            raw = localStorage.getItem(this.storageKey);
        } catch (e) {
            // Blocked storage: read() yields an empty list and write() fails.
            return;
        }
        if (raw !== this.lastRaw) {
            this.items.next(this.read());
        }
    }

    private read(): Bookmark[] {
        this.foreign = [];
        this.readOnly = false;
        let parsed: any;
        try {
            this.lastRaw = localStorage.getItem(this.storageKey);
            parsed = JSON.parse(this.lastRaw);
        } catch (e) {
            return [];
        }
        if (!parsed || !Array.isArray(parsed.items)) {
            return [];
        }
        if (parsed.v !== BOOKMARK_STORAGE_VERSION) {
            this.readOnly = true;
            return parsed.v > BOOKMARK_STORAGE_VERSION
                ? parsed.items.filter(isValidBookmark)
                : [];
        }
        this.foreign = parsed.items.filter(b => !isValidBookmark(b));
        return parsed.items.filter(isValidBookmark);
    }

    private write(items: Bookmark[]): boolean {
        if (!this.storageKey || this.readOnly) {
            return false;
        }
        try {
            const raw = JSON.stringify({
                v: BOOKMARK_STORAGE_VERSION,
                items: [...items, ...this.foreign],
            });
            localStorage.setItem(this.storageKey, raw);
            this.lastRaw = raw;
        } catch (e) {
            return false;
        }
        this.items.next(items);
        return true;
    }
}
