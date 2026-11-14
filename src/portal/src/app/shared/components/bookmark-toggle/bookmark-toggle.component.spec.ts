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
import { SimpleChange } from '@angular/core';
import { SharedTestingModule } from '../../shared.module';
import { BookmarkToggleComponent } from './bookmark-toggle.component';
import {
    BOOKMARK_STORAGE_PREFIX,
    BookmarkKind,
    BookmarkService,
    BookmarkTarget,
} from '../../services/bookmark.service';
import { SessionService } from '../../services/session.service';
import { ErrorHandler } from '../../units/error-handler';

describe('BookmarkToggleComponent', () => {
    let fixture: ComponentFixture<BookmarkToggleComponent>;
    let component: BookmarkToggleComponent;
    let service: BookmarkService;
    let user: any;
    const errorHandler = jasmine.createSpyObj('ErrorHandler', ['error']);
    const target: BookmarkTarget = {
        kind: BookmarkKind.REPOSITORY,
        projectId: 1,
        projectName: 'library',
        repoName: 'nginx',
        defaultLabel: 'nginx',
    };

    function setTarget(t: BookmarkTarget): void {
        component.target = t;
        component.ngOnChanges({ target: new SimpleChange(null, t, false) });
        fixture.detectChanges();
    }

    function button(): HTMLButtonElement {
        return fixture.nativeElement.querySelector('button');
    }

    beforeEach(async () => {
        localStorage.removeItem(BOOKMARK_STORAGE_PREFIX + '7');
        user = { username: 'alice', user_id: 7 };
        await TestBed.configureTestingModule({
            imports: [SharedTestingModule],
            providers: [
                {
                    provide: SessionService,
                    useValue: { getCurrentUser: () => user },
                },
                { provide: ErrorHandler, useValue: errorHandler },
            ],
        }).compileComponents();
        fixture = TestBed.createComponent(BookmarkToggleComponent);
        component = fixture.componentInstance;
        service = TestBed.inject(BookmarkService);
    });

    it('toggles the bookmark on and off', () => {
        setTarget(target);
        expect(button().getAttribute('aria-pressed')).toBe('false');

        button().click();
        fixture.detectChanges();
        expect(service.find(target)).toBeTruthy();
        expect(button().getAttribute('aria-pressed')).toBe('true');

        button().click();
        fixture.detectChanges();
        expect(service.find(target)).toBeUndefined();
        expect(button().getAttribute('aria-pressed')).toBe('false');
    });

    it('reflects bookmarks removed elsewhere, e.g. from the sidebar', () => {
        setTarget(target);
        button().click();
        service.remove(service.find(target).id);
        fixture.detectChanges();
        expect(button().getAttribute('aria-pressed')).toBe('false');
    });

    it('is hidden without a signed-in user or target', () => {
        user = null;
        setTarget(target);
        expect(button()).toBeNull();
        user = { username: 'alice', user_id: 7 };
        setTarget(null);
        expect(button()).toBeNull();
    });

    it('reports the limit separately from a failed save', () => {
        spyOnProperty(service, 'isFull').and.returnValue(true);
        setTarget(target);
        button().click();
        expect(errorHandler.error).toHaveBeenCalledWith(
            'BOOKMARKS.LIMIT_REACHED'
        );
    });

    it('reports a failed add', () => {
        spyOn(service, 'add').and.returnValue(null);
        setTarget(target);
        button().click();
        expect(errorHandler.error).toHaveBeenCalledWith('BOOKMARKS.ADD_FAILED');
    });
});
