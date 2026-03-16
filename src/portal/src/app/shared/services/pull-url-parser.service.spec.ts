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

import { TestBed } from '@angular/core/testing';
import { PullUrlParserService, ParsedPullUrl } from './pull-url-parser.service';

describe('PullUrlParserService', () => {
    let service: PullUrlParserService;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [PullUrlParserService],
        });
        service = TestBed.inject(PullUrlParserService);
    });

    it('should be created', () => {
        expect(service).toBeTruthy();
    });

    describe('parsePullUrl', () => {
        it('should parse simple project/repo:tag format', () => {
            const result = service.parsePullUrl('/myproject/nginx:latest');
            expect(result).toEqual({
                projectName: 'myproject',
                repoName: 'nginx',
            });
        });

        it('should parse project/repo@digest format', () => {
            const result = service.parsePullUrl('/myproject/alpine@sha256:abc123');
            expect(result).toEqual({
                projectName: 'myproject',
                repoName: 'alpine',
            });
        });

        it('should parse nested repository paths', () => {
            const result = service.parsePullUrl('/myproject/nested/repo:v1');
            expect(result).toEqual({
                projectName: 'myproject',
                repoName: 'nested/repo',
            });
        });

        it('should parse deeply nested repository paths', () => {
            const result = service.parsePullUrl(
                '/myproject/a/b/c/repo@sha256:xyz'
            );
            expect(result).toEqual({
                projectName: 'myproject',
                repoName: 'a/b/c/repo',
            });
        });

        it('should handle repo without tag or digest', () => {
            const result = service.parsePullUrl('/myproject/nginx');
            expect(result).toEqual({
                projectName: 'myproject',
                repoName: 'nginx',
            });
        });

        // Excluded paths - should return null
        it('should return null for /harbor/* paths', () => {
            expect(service.parsePullUrl('/harbor/projects')).toBeNull();
            expect(
                service.parsePullUrl('/harbor/projects/123/repositories')
            ).toBeNull();
        });

        it('should return null for /account/* paths', () => {
            expect(service.parsePullUrl('/account/sign-in')).toBeNull();
            expect(service.parsePullUrl('/account/password')).toBeNull();
        });

        it('should return null for /v2/* API paths', () => {
            expect(
                service.parsePullUrl('/v2/myproject/nginx/manifests/latest')
            ).toBeNull();
            expect(service.parsePullUrl('/v2/_catalog')).toBeNull();
        });

        it('should return null for /api/* paths', () => {
            expect(service.parsePullUrl('/api/v2.0/projects')).toBeNull();
        });

        it('should return null for /c/* paths', () => {
            expect(service.parsePullUrl('/c/login')).toBeNull();
        });

        it('should return null for root path', () => {
            expect(service.parsePullUrl('/')).toBeNull();
        });

        it('should return null for empty path', () => {
            expect(service.parsePullUrl('')).toBeNull();
        });

        it('should return null for single segment paths (no repo)', () => {
            expect(service.parsePullUrl('/myproject')).toBeNull();
        });
    });
});
