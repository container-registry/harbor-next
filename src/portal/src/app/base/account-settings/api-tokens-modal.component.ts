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
import { Component, OnInit, ViewChild } from '@angular/core';
import { MessageHandlerService } from '../../shared/services/message-handler.service';
import { SessionService } from '../../shared/services/session.service';
import { UserService } from 'ng-swagger-gen/services/user.service';
import { ProjectService } from 'ng-swagger-gen/services/project.service';
import {
    ConfirmationTargets,
    ConfirmationState,
} from '../../shared/entities/shared.const';
import { ConfirmationDialogComponent } from '../../shared/components/confirmation-dialog';
import { ConfirmationMessage } from '../global-confirmation-dialog/confirmation-message';
import { ConfirmationAcknowledgement } from '../global-confirmation-dialog/confirmation-state-message';

@Component({
    selector: 'api-tokens-modal',
    templateUrl: 'api-tokens-modal.component.html',
    styleUrls: ['./api-tokens-modal.component.scss'],
    standalone: false,
})
export class ApiTokensModalComponent implements OnInit {
    opened = false;
    staticBackdrop = false;

    tokens: any[] = [];
    selectedTokens: any[] = [];
    tokenLoading = false;

    showCreateTokenModal = false;
    createdTokenSecret: string;
    newTokenForm: any = {
        name: '',
        description: '',
        expiresInDays: 0,
    };
    currentUserId: number;

    showRefreshedSecretModal = false;
    refreshedTokenSecret: string;
    refreshingTokenId: number | null = null;

    @ViewChild('confirmationDialog')
    confirmationDialogComponent: ConfirmationDialogComponent;

    // Scope selection for token creation
    scopeProjects: Array<{
        project_id: number;
        project_name: string;
        pull: boolean;
        push: boolean;
    }> = [];
    scopeLoading = false;

    constructor(
        private msgHandler: MessageHandlerService,
        private userService: UserService,
        private projectService: ProjectService,
        private session: SessionService
    ) {}

    /** All projects are selected by default (full auto-computed scope).
     *  False when the project list hasn't loaded yet or failed to load, so a
     *  token created in that state gets no access rather than a
     *  vacuously-true "everything selected" result from an empty array. */
    get allScopeSelected(): boolean {
        return (
            this.scopeProjects.length > 0 &&
            this.scopeProjects.every(p => p.pull && p.push)
        );
    }

    ngOnInit(): void {
        this.currentUserId = this.session.getCurrentUser()?.user_id;
        this.loadTokens();
    }

    open(): void {
        this.opened = true;
        this.loadTokens();
    }

    close(): void {
        this.opened = false;
        this.resetForm();
    }

    loadTokens(): void {
        if (!this.currentUserId) {
            return;
        }
        this.tokenLoading = true;
        this.userService
            .ListPersonalAccessTokens({
                userId: this.currentUserId,
            })
            .subscribe({
                next: tokens => {
                    this.tokens = tokens || [];
                    this.tokens.forEach(token => {
                        token.expired =
                            token.expires_at > 0 &&
                            token.expires_at <= Date.now() / 1000;
                    });
                    this.tokenLoading = false;
                },
                error: () => {
                    this.msgHandler.showError('Failed to load tokens', {});
                    this.tokenLoading = false;
                },
            });
    }

    openCreateTokenModal(): void {
        this.showCreateTokenModal = true;
        this.resetForm();
        this.loadScopeProjects();
    }

    closeCreateTokenModal(): void {
        this.showCreateTokenModal = false;
        this.resetForm();
    }

    loadScopeProjects(): void {
        this.scopeLoading = true;
        this.projectService
            .listProjects({ pageSize: 1000, withDetail: false })
            .subscribe({
                next: (projects: any[]) => {
                    this.scopeProjects = (projects || []).map(p => ({
                        project_id: p.project_id,
                        project_name: p.name,
                        pull: true,
                        push: true,
                    }));
                    this.scopeLoading = false;
                },
                error: () => {
                    this.scopeProjects = [];
                    this.scopeLoading = false;
                },
            });
    }

    selectAllScope(): void {
        this.scopeProjects.forEach(p => {
            p.pull = true;
            p.push = true;
        });
    }

    deselectAllScope(): void {
        this.scopeProjects.forEach(p => {
            p.pull = false;
            p.push = false;
        });
    }

    /** Build the scope JSON string from selected project permissions.
     *  Returns undefined (auto-compute) when all projects have full permissions.
     *  Returns an explicit empty array when nothing is selected, so the token
     *  gets no access rather than falling back to auto-computed full access. */
    buildScopeJson(): string | undefined {
        if (this.allScopeSelected) {
            return undefined;
        }
        const selected = this.scopeProjects.filter(p => p.pull || p.push);
        if (selected.length === 0) {
            return JSON.stringify([]);
        }
        const projectScopes: any[] = selected.map(p => {
            const actions: string[] = [];
            if (p.pull) {
                actions.push('pull');
            }
            if (p.push) {
                actions.push('push');
            }
            return {
                project_id: p.project_id,
                project_name: p.project_name,
                access: [
                    {
                        resource: 'repository',
                        actions: actions,
                    },
                ],
            };
        });
        return JSON.stringify(projectScopes);
    }

    createToken(): void {
        if (!this.newTokenForm.name || !this.currentUserId) {
            this.msgHandler.showError('Token name is required', {});
            return;
        }

        this.tokenLoading = true;
        this.userService
            .CreatePersonalAccessToken({
                userId: this.currentUserId,
                request: {
                    name: this.newTokenForm.name,
                    description: this.newTokenForm.description,
                    expires_in_days: this.newTokenForm.expiresInDays,
                    scope: this.buildScopeJson(),
                },
            })
            .subscribe({
                next: (response: any) => {
                    this.createdTokenSecret = response.secret;
                    this.msgHandler.showSuccess('Token created successfully');
                    this.loadTokens();
                    this.tokenLoading = false;
                },
                error: (err: any) => {
                    if (err && err.status === 409) {
                        this.msgHandler.showError(
                            'Token name already exists',
                            {}
                        );
                    } else {
                        this.msgHandler.showError('Failed to create token', {});
                    }
                    this.tokenLoading = false;
                },
            });
    }

    copyTokenSecret(): void {
        this.copySecretToClipboard(this.createdTokenSecret);
    }

    refreshTokenSecret(tokenId: number): void {
        if (!this.currentUserId || this.refreshingTokenId !== null) {
            return;
        }
        this.refreshingTokenId = tokenId;
        this.userService
            .RefreshPersonalAccessTokenSecret({
                userId: this.currentUserId,
                tokenId: tokenId,
                request: {},
            })
            .subscribe({
                next: (response: any) => {
                    this.refreshingTokenId = null;
                    this.refreshedTokenSecret = response.secret;
                    this.showRefreshedSecretModal = true;
                    this.msgHandler.showSuccess(
                        'Token secret refreshed successfully'
                    );
                    this.loadTokens();
                },
                error: () => {
                    this.refreshingTokenId = null;
                    this.msgHandler.showError(
                        'Failed to refresh token secret',
                        {}
                    );
                },
            });
    }

    closeRefreshedSecretModal(): void {
        this.showRefreshedSecretModal = false;
        this.refreshedTokenSecret = '';
    }

    copyRefreshedTokenSecret(): void {
        this.copySecretToClipboard(this.refreshedTokenSecret);
    }

    private copySecretToClipboard(secret: string): void {
        if (navigator.clipboard) {
            navigator.clipboard
                .writeText(secret)
                .then(() => {
                    this.msgHandler.showSuccess('Token copied to clipboard');
                })
                .catch(() => {
                    this.msgHandler.showError('Failed to copy token');
                });
        } else {
            const copyInput = document.createElement('textarea');
            copyInput.value = secret;
            document.body.appendChild(copyInput);
            copyInput.select();
            document.execCommand('copy');
            document.body.removeChild(copyInput);
            this.msgHandler.showSuccess('Token copied to clipboard');
        }
    }

    revokeToken(tokenId: number): void {
        if (!this.currentUserId) {
            return;
        }
        const token = this.tokens.find(t => t.id === tokenId);
        if (!token) {
            return;
        }
        this.userService
            .UpdatePersonalAccessToken({
                userId: this.currentUserId,
                tokenId: tokenId,
                request: {
                    disabled: !token.disabled,
                },
            })
            .subscribe({
                next: () => {
                    this.msgHandler.showSuccess('Token updated successfully');
                    this.loadTokens();
                },
                error: () => {
                    this.msgHandler.showError('Failed to update token', {});
                },
            });
    }

    deleteToken(tokenId: number): void {
        const deleteTokenMessage: ConfirmationMessage = new ConfirmationMessage(
            'PROFILE.DELETE_PAT_TITLE',
            'PROFILE.DELETE_PAT_CONFIRM',
            '',
            tokenId,
            ConfirmationTargets.USER_PAT
        );
        this.confirmationDialogComponent.open(deleteTokenMessage);
    }

    confirmDeleteToken(message: ConfirmationAcknowledgement): void {
        if (
            !message ||
            message.state !== ConfirmationState.CONFIRMED ||
            message.source !== ConfirmationTargets.USER_PAT ||
            !this.currentUserId
        ) {
            return;
        }
        const tokenId: number = message.data;
        this.userService
            .DeletePersonalAccessToken({
                userId: this.currentUserId,
                tokenId: tokenId,
            })
            .subscribe({
                next: () => {
                    this.msgHandler.showSuccess('Token deleted successfully');
                    this.loadTokens();
                },
                error: () => {
                    this.msgHandler.showError('Failed to delete token', {});
                },
            });
    }

    private resetForm(): void {
        this.newTokenForm = {
            name: '',
            description: '',
            expiresInDays: 0,
        };
        this.createdTokenSecret = '';
    }
}
