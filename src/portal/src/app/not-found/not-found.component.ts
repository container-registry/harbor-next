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
import { Component, OnInit, OnDestroy } from '@angular/core';
import { Router } from '@angular/router';
import { PullUrlParserService } from '../shared/services/pull-url-parser.service';
import { ProjectService } from '../shared/services/project.service';

const defaultInterval = 1000;
const defaultLeftTime = 5;

@Component({
    selector: 'page-not-found',
    templateUrl: 'not-found.component.html',
    styleUrls: ['not-found.component.scss'],
})
export class PageNotFoundComponent implements OnInit, OnDestroy {
    leftSeconds: number = defaultLeftTime;
    timeInterval: any = null;
    isCheckingPullUrl: boolean = true;

    constructor(
        private router: Router,
        private pullUrlParser: PullUrlParserService,
        private projectService: ProjectService
    ) {}

    ngOnInit(): void {
        this.tryRedirectFromPullUrl();
    }

    /**
     * Try to parse the current URL as a pull URL and redirect to the project.
     * If parsing fails or project lookup fails, show the 404 page.
     */
    private tryRedirectFromPullUrl(): void {
        const parsed = this.pullUrlParser.parsePullUrl(
            window.location.pathname
        );

        if (!parsed) {
            // Not a pull URL - show normal 404
            this.showNotFound();
            return;
        }

        // Try to look up the project by name
        this.projectService.getProject(parsed.projectName).subscribe({
            next: project => {
                // Success - redirect to project repository
                this.router.navigate([
                    '/harbor',
                    'projects',
                    project.project_id,
                    'repositories',
                    parsed.repoName,
                    'artifacts-tab',
                ]);
            },
            error: () => {
                // Any error (401/403/404/500) - show 404
                // Don't reveal whether project exists or user lacks permission
                this.showNotFound();
            },
        });
    }

    /**
     * Show the 404 page with countdown to redirect.
     */
    private showNotFound(): void {
        this.isCheckingPullUrl = false;
        this.startCountdown();
    }

    /**
     * Start the countdown timer to redirect to harbor home.
     */
    private startCountdown(): void {
        if (!this.timeInterval) {
            this.timeInterval = setInterval(() => {
                this.leftSeconds--;
                if (this.leftSeconds <= 0) {
                    this.router.navigate(['harbor']);
                    clearInterval(this.timeInterval);
                }
            }, defaultInterval);
        }
    }

    ngOnDestroy(): void {
        if (this.timeInterval) {
            clearInterval(this.timeInterval);
        }
    }
}
