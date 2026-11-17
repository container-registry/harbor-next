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
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Component } from '@angular/core';
import { By } from '@angular/platform-browser';
import { Router } from '@angular/router';
import { Location } from '@angular/common';
import { BehaviorSubject } from 'rxjs';
import { SharedTestingModule } from '../../../shared/shared.module';
import {
    Bookmark,
    BOOKMARK_STORAGE_PREFIX,
    BookmarkKind,
    BookmarkService,
    BookmarkTarget,
} from '../../../shared/services/bookmark.service';
import {
    BookmarkHealthService,
    BookmarkStatus,
} from '../../../shared/services/bookmark-health.service';
import { SessionService } from '../../../shared/services/session.service';
import {
    SidebarBookmarksComponent,
    groupByProject,
    reorderInGroup,
} from './sidebar-bookmarks.component';

// The nav group needs the services its parent clr-vertical-nav provides.
@Component({
    template: `<clr-vertical-nav>
        <clr-vertical-nav-group>
            <clr-vertical-nav-group-children *clrIfExpanded="true">
                <sidebar-bookmarks></sidebar-bookmarks>
            </clr-vertical-nav-group-children>
        </clr-vertical-nav-group>
    </clr-vertical-nav>`,
})
class HostComponent {}

describe('SidebarBookmarksComponent', () => {
    let fixture: ComponentFixture<HostComponent>;
    let component: SidebarBookmarksComponent;
    let service: BookmarkService;
    let statuses: BehaviorSubject<ReadonlyMap<string, BookmarkStatus>>;
    let health: jasmine.SpyObj<BookmarkHealthService>;
    const library: BookmarkTarget = {
        kind: BookmarkKind.PROJECT,
        projectId: 1,
        projectName: 'library',
        defaultLabel: 'library',
    };
    const nginxTag: BookmarkTarget = {
        kind: BookmarkKind.TAG,
        projectId: 1,
        projectName: 'library',
        repoName: 'nginx',
        tag: 'latest',
        defaultLabel: 'nginx:latest',
    };
    const other: BookmarkTarget = {
        kind: BookmarkKind.REPOSITORY,
        projectId: 2,
        projectName: 'team',
        repoName: 'app',
        defaultLabel: 'app',
    };

    function rows(): HTMLElement[] {
        return Array.from(
            fixture.nativeElement.querySelectorAll('.bookmark-row')
        );
    }

    function render(): void {
        fixture.detectChanges();
        component = fixture.debugElement.query(
            By.directive(SidebarBookmarksComponent)
        ).componentInstance;
    }

    beforeEach(async () => {
        localStorage.removeItem(BOOKMARK_STORAGE_PREFIX + '7');
        statuses = new BehaviorSubject<ReadonlyMap<string, BookmarkStatus>>(
            new Map()
        );
        health = jasmine.createSpyObj(
            'BookmarkHealthService',
            ['checkAll', 'check', 'statusOf'],
            { statuses$: statuses.asObservable() }
        );
        health.statusOf.and.callFake(
            (s, b: Bookmark) =>
                s.get((b as Bookmark).id) || BookmarkStatus.UNKNOWN
        );
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            declarations: [HostComponent, SidebarBookmarksComponent],
            providers: [
                {
                    provide: SessionService,
                    useValue: {
                        getCurrentUser: () => ({
                            username: 'alice',
                            user_id: 7,
                        }),
                    },
                },
                { provide: BookmarkHealthService, useValue: health },
            ],
        }).compileComponents();
        service = TestBed.inject(BookmarkService);
        fixture = TestBed.createComponent(HostComponent);
    });

    it('renders no rows while there are no bookmarks', () => {
        render();
        expect(rows().length).toBe(0);
        expect(health.checkAll).toHaveBeenCalledWith([]);
    });

    it('groups bookmarks by project in first-seen order', () => {
        service.load();
        service.add(library);
        service.add(other);
        service.add(nginxTag);
        render();
        const titles = Array.from(
            fixture.nativeElement.querySelectorAll('.bookmark-group-title')
        ).map((e: HTMLElement) => e.textContent.trim());
        expect(titles).toEqual(['library', 'team']);
        expect(
            rows().map(r => r.querySelector('.bookmark-label').textContent)
        ).toEqual(['library', 'nginx:latest', 'app']);
        expect(health.checkAll).toHaveBeenCalledWith(service.bookmarks);
    });

    it('links tag bookmarks to the tag redirect route', () => {
        service.load();
        service.add(nginxTag);
        render();
        const href = rows()[0].querySelector('a').getAttribute('href');
        expect(href).toContain(
            '/harbor/projects/1/repositories/nginx/artifacts-tab/tags/latest'
        );
    });

    it('navigates to the bookmark and re-checks it when a guard bounces the click', async () => {
        const router = TestBed.inject(Router);
        const navigate = spyOn(router, 'navigate').and.resolveTo(true);
        service.load();
        service.add({
            ...other,
            kind: BookmarkKind.REPOSITORY_FILTER,
            query: 'ngi',
        });
        render();
        rows()[0].querySelector('a').click();
        expect(navigate).toHaveBeenCalledWith(
            ['/harbor/projects', 2, 'repositories'],
            { queryParams: { name: 'ngi' } }
        );
        await fixture.whenStable();
        expect(health.check).not.toHaveBeenCalled();

        navigate.and.resolveTo(false);
        rows()[0].querySelector('a').click();
        await fixture.whenStable();
        expect(health.check).toHaveBeenCalledWith([service.bookmarks[0]]);
    });

    it('ignores Enter and Escape while an IME composition is open', async () => {
        service.load();
        service.add(library);
        render();
        component.startEdit(service.bookmarks[0]);
        render();
        const input: HTMLInputElement = rows()[0].querySelector('input');
        await fixture.whenStable();
        input.value = '북마';
        input.dispatchEvent(new Event('input'));
        input.dispatchEvent(
            new KeyboardEvent('keydown', { key: 'Enter', isComposing: true })
        );
        input.dispatchEvent(
            new KeyboardEvent('keydown', { key: 'Escape', isComposing: true })
        );
        render();
        expect(rows()[0].querySelector('input')).not.toBeNull();
        expect(service.bookmarks[0].label).toBe('library');
    });

    it('renames inline with Enter and cancels with Escape', async () => {
        service.load();
        const b = service.add(library);
        render();
        (rows()[0].querySelector('.bookmark-action') as HTMLElement).click();
        render();
        let input: HTMLInputElement = rows()[0].querySelector('input');
        await fixture.whenStable();
        input.value = 'Main';
        input.dispatchEvent(new Event('input'));
        input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
        render();
        expect(service.bookmarks[0].label).toBe('Main');
        expect(rows()[0].querySelector('input')).toBeNull();

        component.startEdit(service.bookmarks[0]);
        render();
        input = rows()[0].querySelector('input');
        await fixture.whenStable();
        input.value = 'ignored';
        input.dispatchEvent(new Event('input'));
        input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
        render();
        expect(service.find(library).label).toBe('Main');
        expect(service.find(library).id).toBe(b.id);
    });

    it('deletes from the sidebar', () => {
        service.load();
        service.add(library);
        service.add(other);
        render();
        (
            rows()[0].querySelectorAll('.bookmark-action')[1] as HTMLElement
        ).click();
        render();
        expect(service.bookmarks.map(b => b.projectId)).toEqual([2]);
        expect(rows().length).toBe(1);
    });

    it('flags missing targets with a question mark and keeps them', () => {
        service.load();
        const gone = service.add(nginxTag);
        const ok = service.add(library);
        statuses.next(
            new Map([
                [gone.id, BookmarkStatus.MISSING],
                [ok.id, BookmarkStatus.OK],
            ])
        );
        render();
        const marks = rows().map(r => r.querySelector('.bookmark-broken'));
        expect(service.bookmarks.length).toBe(2);
        expect(marks[0].textContent.trim()).toBe('?');
        expect(marks[1]).toBeNull();
    });

    it('reorders by drag and drop only inside a project', () => {
        service.load();
        const a = service.add(library);
        const c = service.add(other);
        const b = service.add(nginxTag);
        render();
        const data = new DataTransfer();
        component.onDragStart(
            new DragEvent('dragstart', { dataTransfer: data }),
            a
        );
        const over = new DragEvent('dragover', {
            dataTransfer: data,
            cancelable: true,
        });
        component.onDragOver(over, c);
        expect(over.defaultPrevented).toBeFalse();

        component.onDragOver(
            new DragEvent('dragover', { dataTransfer: data }),
            b
        );
        // Dragging down lands below the target, and the marker says so.
        expect(component.dropAfter).toBeTrue();
        component.onDrop(new DragEvent('drop', { dataTransfer: data }), b);
        expect(service.bookmarks.map(x => x.id)).toEqual([b.id, c.id, a.id]);
        expect(groupByProject(service.bookmarks).map(g => g.projectId)).toEqual(
            [1, 2]
        );
    });

    it('reorderInGroup moves within the project and keeps other slots', () => {
        const mk = (id: string, projectId: number) =>
            ({ id, projectId, projectName: 'p' + projectId } as Bookmark);
        const list = [mk('a', 1), mk('x', 2), mk('b', 1), mk('c', 1)];
        expect(reorderInGroup(list, 'a', 'b')).toEqual(['b', 'x', 'a', 'c']);
        expect(reorderInGroup(list, 'c', 'a')).toEqual(['c', 'x', 'a', 'b']);
        expect(reorderInGroup(list, 'a', 'x')).toEqual(['a', 'x', 'b', 'c']);
    });

    it('highlights project and repository bookmarks on their redirect targets', () => {
        const path = spyOn(TestBed.inject(Location), 'path');
        service.load();
        const project = service.add(library);
        const repo = service.add({
            ...other,
            projectId: 1,
            projectName: 'library',
            repoName: 'team/app',
        });
        const tag = service.add(nginxTag);
        path.and.returnValue('/harbor/projects/1/repositories');
        render();
        expect(component.activeId).toBe(project.id);

        path.and.returnValue(
            '/harbor/projects/1/repositories/team%2Fapp/artifacts-tab'
        );
        service.rename(tag.id, 'refresh');
        render();
        expect(component.activeId).toBe(repo.id);

        path.and.returnValue(
            '/harbor/projects/1/repositories/nginx/artifacts-tab/tags/latest'
        );
        service.rename(tag.id, 'refresh again');
        render();
        expect(component.activeId).toBeUndefined();
    });

    it('groupByProject keeps storage order inside a group', () => {
        const mk = (id: string, projectId: number) =>
            ({ id, projectId, projectName: 'p' + projectId } as Bookmark);
        const groups = groupByProject([mk('1', 1), mk('2', 2), mk('3', 1)]);
        expect(groups.map(g => g.items.map(i => i.id))).toEqual([
            ['1', '3'],
            ['2'],
        ]);
    });
});
