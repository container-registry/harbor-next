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
    Component,
    Input,
    OnChanges,
    OnDestroy,
    SimpleChanges,
} from '@angular/core';
import { Subscription } from 'rxjs';
import {
    Bookmark,
    BookmarkService,
    BookmarkTarget,
    bookmarkKey,
} from '../../services/bookmark.service';
import { ErrorHandler } from '../../units/error-handler';

@Component({
    selector: 'bookmark-toggle',
    templateUrl: './bookmark-toggle.component.html',
    styleUrls: ['./bookmark-toggle.component.scss'],
})
export class BookmarkToggleComponent implements OnChanges, OnDestroy {
    @Input() target: BookmarkTarget;
    @Input() size: number = 20;
    @Input() addTitleKey: string = 'BOOKMARKS.ADD';
    bookmark: Bookmark;
    enabled: boolean = false;
    private watchedKey: string;
    private sub: Subscription;

    constructor(
        private bookmarkService: BookmarkService,
        private errorHandler: ErrorHandler
    ) {}

    ngOnChanges(changes: SimpleChanges): void {
        if (changes['target']) {
            this.subscribe();
        }
    }

    ngOnDestroy(): void {
        this.sub?.unsubscribe();
    }

    toggle(event: Event): void {
        event.stopPropagation();
        event.preventDefault();
        if (this.bookmark) {
            this.bookmarkService.remove(this.bookmark.id);
        } else if (this.bookmarkService.isFull) {
            this.errorHandler.error('BOOKMARKS.LIMIT_REACHED');
        } else if (!this.bookmarkService.add(this.target)) {
            this.errorHandler.error('BOOKMARKS.ADD_FAILED');
        }
    }

    private subscribe(): void {
        this.enabled = !!this.target && this.bookmarkService.load();
        const key = this.enabled ? bookmarkKey(this.target) : null;
        if (key === this.watchedKey) {
            return;
        }
        this.watchedKey = key;
        this.sub?.unsubscribe();
        this.sub = null;
        this.bookmark = undefined;
        if (key) {
            this.sub = this.bookmarkService
                .watch(this.target)
                .subscribe(b => (this.bookmark = b));
        }
    }
}
