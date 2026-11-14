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
    ChangeDetectorRef,
    Component,
    ElementRef,
    OnDestroy,
    OnInit,
    ViewChild,
} from '@angular/core';
import { Location } from '@angular/common';
import { NavigationEnd, Params, Router } from '@angular/router';
import { combineLatest, Subscription } from 'rxjs';
import { filter } from 'rxjs/operators';
import {
    Bookmark,
    BookmarkKind,
    BookmarkService,
    MAX_BOOKMARK_LABEL_LENGTH,
    bookmarkLink,
} from '../../../shared/services/bookmark.service';
import {
    BookmarkHealthService,
    BookmarkStatus,
} from '../../../shared/services/bookmark-health.service';

export interface BookmarkGroup {
    projectId: number;
    projectName: string;
    items: Bookmark[];
}

const KIND_ICONS: Record<BookmarkKind, string> = {
    [BookmarkKind.PROJECT]: 'organization',
    [BookmarkKind.REPOSITORY]: 'container',
    [BookmarkKind.TAG]: 'tag',
    [BookmarkKind.REPOSITORY_FILTER]: 'filter',
    [BookmarkKind.ARTIFACT_FILTER]: 'filter',
};

// Groups keep the order in which their project first appears in storage.
export function groupByProject(bookmarks: Bookmark[]): BookmarkGroup[] {
    const groups = new Map<number, BookmarkGroup>();
    bookmarks.forEach(b => {
        let group = groups.get(b.projectId);
        if (!group) {
            group = {
                projectId: b.projectId,
                projectName: b.projectName,
                items: [],
            };
            groups.set(b.projectId, group);
        }
        group.items.push(b);
    });
    return Array.from(groups.values());
}

// Moves a bookmark to the target's place within its project. The project's
// bookmarks keep their storage slots, so the order of the groups never changes.
export function reorderInGroup(
    bookmarks: Bookmark[],
    draggedId: string,
    targetId: string
): string[] {
    const ids = bookmarks.map(b => b.id);
    const dragged = bookmarks.find(b => b.id === draggedId);
    if (!dragged || draggedId === targetId) {
        return ids;
    }
    const slots: number[] = [];
    bookmarks.forEach((b, i) => {
        if (b.projectId === dragged.projectId) {
            slots.push(i);
        }
    });
    const group = slots.map(i => ids[i]);
    const to = group.indexOf(targetId);
    if (to === -1) {
        return ids;
    }
    group.splice(group.indexOf(draggedId), 1);
    group.splice(to, 0, draggedId);
    slots.forEach((slot, i) => (ids[slot] = group[i]));
    return ids;
}

@Component({
    selector: 'sidebar-bookmarks',
    templateUrl: './sidebar-bookmarks.component.html',
    styleUrls: ['./sidebar-bookmarks.component.scss'],
})
export class SidebarBookmarksComponent implements OnInit, OnDestroy {
    readonly maxLabelLength = MAX_BOOKMARK_LABEL_LENGTH;
    groups: BookmarkGroup[] = [];
    private hrefs = new Map<string, string>();
    private activePaths = new Map<string, string>();
    private statuses: ReadonlyMap<string, BookmarkStatus> = new Map();
    activeId: string;
    editingId: string;
    editLabel: string;
    draggingId: string;
    dropTargetId: string;
    dropAfter = false;
    private bookmarks: Bookmark[] = [];
    private subs: Subscription[] = [];

    @ViewChild('editInput')
    set editInput(el: ElementRef<HTMLInputElement>) {
        if (el) {
            el.nativeElement.focus();
            // ngModel writes the value after this setter; selecting earlier is lost.
            setTimeout(() => el.nativeElement.select());
        }
    }

    constructor(
        private bookmarkService: BookmarkService,
        private health: BookmarkHealthService,
        private router: Router,
        private location: Location,
        private cd: ChangeDetectorRef
    ) {}

    ngOnInit(): void {
        this.bookmarkService.load();
        this.subs.push(
            combineLatest([
                this.bookmarkService.bookmarks$,
                this.health.statuses$,
            ]).subscribe(([bookmarks, statuses]) => {
                this.statuses = statuses;
                if (bookmarks !== this.bookmarks) {
                    this.setBookmarks(bookmarks);
                }
            }),
            this.router.events
                .pipe(filter(e => e instanceof NavigationEnd))
                .subscribe(() => {
                    this.updateActive();
                    this.health.checkAll(this.bookmarks);
                })
        );
        this.health.checkAll(this.bookmarks);
    }

    ngOnDestroy(): void {
        this.subs.forEach(s => s.unsubscribe());
    }

    hrefOf(b: Bookmark): string {
        return this.hrefs.get(b.id);
    }

    icon(b: Bookmark): string {
        return KIND_ICONS[b.kind];
    }

    status(b: Bookmark): BookmarkStatus {
        return this.health.statusOf(this.statuses, b);
    }

    isBroken(b: Bookmark): boolean {
        const s = this.status(b);
        return s === BookmarkStatus.MISSING || s === BookmarkStatus.FORBIDDEN;
    }

    brokenMessageKey(b: Bookmark): string {
        return this.status(b) === BookmarkStatus.FORBIDDEN
            ? 'BOOKMARKS.FORBIDDEN'
            : 'BOOKMARKS.MISSING';
    }

    open(event: MouseEvent, b: Bookmark): void {
        if (
            event.button !== 0 ||
            event.ctrlKey ||
            event.metaKey ||
            event.shiftKey ||
            event.altKey
        ) {
            return;
        }
        event.preventDefault();
        const link = bookmarkLink(b);
        this.router
            .navigate(link.commands, { queryParams: link.queryParams })
            .then(ok => {
                // A guard bounced the click, e.g. the project is gone or access was revoked.
                if (ok === false) {
                    this.health.check([b]);
                }
            });
    }

    // Enter and Escape also confirm or cancel an IME composition.
    onEditKey(event: KeyboardEvent, save: boolean): void {
        if (event.isComposing || event.keyCode === 229) {
            return;
        }
        if (save) {
            this.saveEdit();
        } else {
            this.cancelEdit();
        }
    }

    startEdit(b: Bookmark): void {
        this.editingId = b.id;
        this.editLabel = b.label;
    }

    saveEdit(): void {
        if (this.editingId) {
            this.bookmarkService.rename(this.editingId, this.editLabel);
            this.editingId = null;
        }
    }

    cancelEdit(): void {
        this.editingId = null;
    }

    remove(b: Bookmark): void {
        if (this.editingId === b.id) {
            this.editingId = null;
        }
        this.bookmarkService.remove(b.id);
    }

    onDragStart(event: DragEvent, b: Bookmark): void {
        if (this.editingId) {
            event.preventDefault();
            return;
        }
        this.draggingId = b.id;
        event.dataTransfer.effectAllowed = 'move';
        event.dataTransfer.setData('text/plain', b.id);
    }

    onDragOver(event: DragEvent, b: Bookmark): void {
        if (this.canDrop(b)) {
            event.preventDefault();
            event.dataTransfer.dropEffect = 'move';
            this.dropTargetId = b.id;
            this.dropAfter = this.isBelow(this.draggingId, b);
        }
    }

    onDrop(event: DragEvent, target: Bookmark): void {
        event.preventDefault();
        if (this.canDrop(target)) {
            this.bookmarkService.reorder(
                reorderInGroup(this.bookmarks, this.draggingId, target.id)
            );
        }
        this.onDragEnd();
    }

    onDragEnd(): void {
        this.draggingId = null;
        this.dropTargetId = null;
        this.dropAfter = false;
    }

    trackGroup(_: number, g: BookmarkGroup): number {
        return g.projectId;
    }

    trackBookmark(_: number, b: Bookmark): string {
        return b.id;
    }

    private isBelow(draggedId: string, target: Bookmark): boolean {
        const ids = this.bookmarks
            .filter(b => b.projectId === target.projectId)
            .map(b => b.id);
        return ids.indexOf(draggedId) < ids.indexOf(target.id);
    }

    private canDrop(target: Bookmark): boolean {
        if (!this.draggingId || this.draggingId === target.id) {
            return false;
        }
        const dragged = this.bookmarks.find(b => b.id === this.draggingId);
        return !!dragged && dragged.projectId === target.projectId;
    }

    private setBookmarks(bookmarks: Bookmark[]): void {
        this.bookmarks = bookmarks;
        this.groups = groupByProject(bookmarks);
        this.hrefs = new Map(bookmarks.map(b => [b.id, this.href(b)]));
        this.activePaths = new Map(
            bookmarks
                .map(b => [b.id, this.activePath(b)] as [string, string])
                .filter(([, path]) => !!path)
        );
        this.updateActive();
        this.cd.markForCheck();
    }

    private href(b: Bookmark): string {
        const link = bookmarkLink(b);
        return this.location.prepareExternalUrl(
            this.serialize(link.commands, link.queryParams)
        );
    }

    // The URL a bookmark ends up on after the router's redirects. Tag bookmarks
    // land on a digest page that several tags share, so they never show as active.
    private activePath(b: Bookmark): string | null {
        const link = bookmarkLink(b);
        switch (b.kind) {
            case BookmarkKind.PROJECT:
                return this.serialize([...link.commands, 'repositories']);
            case BookmarkKind.REPOSITORY:
                return this.serialize([...link.commands, 'artifacts-tab']);
            case BookmarkKind.TAG:
                return null;
            default:
                return this.serialize(link.commands, link.queryParams);
        }
    }

    private serialize(commands: any[], queryParams?: Params): string {
        return this.router.serializeUrl(
            this.router.createUrlTree(commands, { queryParams })
        );
    }

    private updateActive(): void {
        const current = this.location.path();
        this.activeId = this.bookmarks.find(
            b => this.activePaths.get(b.id) === current
        )?.id;
    }
}
