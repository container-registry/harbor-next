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

import { Injectable } from '@angular/core';

export interface ParsedPullUrl {
    projectName: string;
    repoName: string;
}

// Paths that should not be treated as pull URLs
const EXCLUDED_PATH_PREFIXES = [
    '/harbor',
    '/account',
    '/v2',
    '/api',
    '/c',
    '/oidc-onboard',
    '/license',
    '/devcenter-api-2.0',
];

@Injectable({
    providedIn: 'root',
})
export class PullUrlParserService {
    /**
     * Parse a URL path to extract project and repository information.
     * Returns null if the path is not a valid pull URL.
     *
     * Valid formats:
     *   /project/repo
     *   /project/repo:tag
     *   /project/repo@sha256:digest
     *   /project/nested/repo:tag
     *
     * @param path The URL pathname (e.g., '/myproject/nginx:latest')
     * @returns ParsedPullUrl or null if not a valid pull URL
     */
    parsePullUrl(path: string): ParsedPullUrl | null {
        if (!path || path === '/') {
            return null;
        }

        // Check if path starts with excluded prefixes
        for (const prefix of EXCLUDED_PATH_PREFIXES) {
            if (path.startsWith(prefix + '/') || path === prefix) {
                return null;
            }
        }

        // Remove leading slash
        const cleanPath = path.startsWith('/') ? path.substring(1) : path;

        if (!cleanPath) {
            return null;
        }

        // Split by first slash to get project name
        const firstSlashIndex = cleanPath.indexOf('/');
        if (firstSlashIndex === -1) {
            // No slash means only project name, no repo - invalid pull URL
            return null;
        }

        const projectName = cleanPath.substring(0, firstSlashIndex);
        let repoPath = cleanPath.substring(firstSlashIndex + 1);

        if (!projectName || !repoPath) {
            return null;
        }

        // Remove tag (:tag) or digest (@sha256:...) suffix from repo name
        const tagIndex = repoPath.lastIndexOf(':');
        const digestIndex = repoPath.lastIndexOf('@');

        if (digestIndex > 0) {
            // Has digest - remove everything after @
            repoPath = repoPath.substring(0, digestIndex);
        } else if (tagIndex > 0) {
            // Has tag - remove everything after :
            repoPath = repoPath.substring(0, tagIndex);
        }

        return {
            projectName,
            repoName: repoPath,
        };
    }
}
