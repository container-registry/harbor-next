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
    EventEmitter,
    Input,
    OnChanges,
    Output,
} from '@angular/core';
import {
    SecretValidationError,
    SecretValidator,
} from '../../entities/secret-validator';

@Component({
    selector: 'robot-secret-input',
    templateUrl: './robot-secret-input.component.html',
    styleUrls: ['./robot-secret-input.component.scss'],
})
export class RobotSecretInputComponent implements OnChanges {
    @Input() secret: string = '';
    @Output() secretChange = new EventEmitter<string>();
    @Output() validChange = new EventEmitter<boolean>();

    confirmSecret: string = '';
    showPassword: boolean = false;
    validationErrors: SecretValidationError[] = [];
    isDirty: boolean = false;

    ngOnChanges(): void {
        // an external reset() of the bound secret (e.g. wizard cancel/reset)
        // should also clear this component's own local state
        if (!this.secret) {
            this.confirmSecret = '';
            this.showPassword = false;
            this.validationErrors = [];
            this.isDirty = false;
        }
        this.emitValidity();
    }

    onSecretInput(value: string): void {
        this.secret = value;
        this.secretChange.emit(value);
        this.validate();
    }

    onConfirmInput(): void {
        this.emitValidity();
    }

    validate(): void {
        this.isDirty = true;
        this.validationErrors = this.secret
            ? SecretValidator.getValidationErrors(this.secret)
            : [];
        this.emitValidity();
    }

    toggleVisibility(): void {
        this.showPassword = !this.showPassword;
    }

    isSecretValid(): boolean {
        return !!this.secret && SecretValidator.validate(this.secret).isValid;
    }

    secretsMatch(): boolean {
        return (
            !!this.secret &&
            !!this.confirmSecret &&
            this.secret === this.confirmSecret
        );
    }

    isAcceptable(): boolean {
        return !this.secret || (this.isSecretValid() && this.secretsMatch());
    }

    private emitValidity(): void {
        this.validChange.emit(this.isAcceptable());
    }

    protected readonly SecretValidator = SecretValidator;
}
