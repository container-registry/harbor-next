import { Component, OnInit, EventEmitter, Output, Renderer2, ElementRef } from "@angular/core";
import { DomSanitizer, SafeResourceUrl } from "@angular/platform-browser";
import { SessionService } from "../../services/session.service";
import { SkinableConfig } from "../../../services/skinable-config.service";
import { environment } from "../../../../environments/environment";

const iframeDisplayHidden = "none";
const iframeDisplayVisible = "";
const defaultIframeWidth = "400px";
const defaultIframeHeight = "550px";
const checkout = `https://${environment.chargebeeUrl}.chargebee.com/hosted_pages/checkout?subscription_items[item_price_id][0]=SubscriptionName-Currency-PlanPeriod&subscription_items[quantity][0]=1&layout=full_page&subscription[cf_Project_Name]=projectName`
const customerPortal = `https://${environment.chargebeeUrl}.chargebeeportal.com`

@Component({
    selector: "chargebee-dialog",
    templateUrl: "./chargebee-dialog.component.html",
    styleUrls: ["./chargebee-dialog.component.scss"]
})
export class ChargebeeDialogComponent implements OnInit {
    opened = false;
    loaded = false;
    size = "full-screen";
    iframeWidth = "0px";
    iframeHeight = "0px";
    iframeDisplay = iframeDisplayHidden;
    iframeSrc: SafeResourceUrl = customerPortal;
    sanitizedFrameSrc: SafeResourceUrl = this.sanitizer.bypassSecurityTrustResourceUrl(customerPortal);
    currentUser = null;
    subscriptionName = "c8n-subscription";
    projectName = null;
    isSubscribed = true;
    selectedPlan: "Monthly" | "Yearly" = "Yearly";
    currency = "EUR";
    isSubscriptionSuccessful = false;
    showSubscriptionCheckoutOption = true;
    userLocation = null;
    deploymentConfig = null;
    subscriptionStatusMonitoring = null;
    title = "Container Registry Subscription";
    hasGithubApp = false;

    @Output() frameEvent = new EventEmitter<any>();
    constructor(
        private renderer: Renderer2,
        private el: ElementRef,
        private session: SessionService,
        private sanitizer: DomSanitizer,
        private skinableConfig: SkinableConfig,
    ) {}

    ngOnInit(): void {
        this.opened = false;
        this.setupMessageListener();
    }

    formatPrice(currency, number, digits=2) {
        return new Intl.NumberFormat("en-US", {
            style: "currency",
            currency,
            maximumFractionDigits: digits,
        }).format(number);
    }

    monitorSubscriptionStatus(): void {
        this.session.getUserSubscriptionStatus(this.currentUser.email).subscribe(data => {
            if (data != null) {
                this.projectName = data.projectName;
                // when a user just subscribed
                if (!this.isSubscribed && data.subscribed) {
                    this.isSubscriptionSuccessful = true;
                    this.title = "Enjoy Your Container Registry!"
                // while a user is doing subscribing process
                } else {
                    this.isSubscriptionSuccessful = false;
                    this.title = "Container Registry Subscription";
                }
                this.isSubscribed = data.subscribed;
                this.hasGithubApp = data.hasGithubApp;
            }
            var newIframeSrc = null;
            // if a user is subscribe, only show portal
            if (this.isSubscribed || this.hasGithubApp) {
                this.stopSubscriptionStatusMonitoring();
                newIframeSrc = customerPortal;
            // else show checkout
            } else {
                newIframeSrc = this.getCheckoutSrc();
            }
            this.setIframe(newIframeSrc);
        });
    }
    private setIframe(newIframeSrc: string): void {
        if (newIframeSrc == this.iframeSrc) return;
        this.iframeSrc = newIframeSrc;
        this.sanitizedFrameSrc = this.sanitizer.bypassSecurityTrustResourceUrl(newIframeSrc);
    }
    private startSubscriptionStatusMonitoring(): void {
        this.subscriptionStatusMonitoring = setInterval(() => {
            this.monitorSubscriptionStatus();
        }, 1000);
    }

    private stopSubscriptionStatusMonitoring(): void {
        if (this.subscriptionStatusMonitoring) {
            clearInterval(this.subscriptionStatusMonitoring);
        }
    }

    open(showSubscriptionCheckoutOption: boolean): void {
        this.opened = true;
        this.showSubscriptionCheckoutOption = showSubscriptionCheckoutOption;
        if (!this.showSubscriptionCheckoutOption) {
            return;
        }
        if (this.userLocation == null) {
            const location = this.session.getUserLocation().subscribe(data => {
                this.userLocation = data;
            });
        }
        this.currentUser = this.session.getCurrentUser();
        this.startSubscriptionStatusMonitoring();
    }

    onModalOpenChange(opened: boolean) {
        this.stopSubscriptionStatusMonitoring();
        this.iframeDisplay = iframeDisplayHidden;
        this.loaded = false;
        this.isSubscribed = true;
        this.isSubscriptionSuccessful = false;
        this.selectedPlan = "Yearly";
        this.title = "Container Registry Subscription";
        this.iframeWidth = "0px";
        this.iframeHeight = "0px";
        this.hasGithubApp = false;
    }

    getCheckoutSrc() {
        this.currency = this.userLocation.currency.toUpperCase();
        return checkout
                    .replace("PlanPeriod", this.selectedPlan)
                    .replace("Currency", this.userLocation.currency.toUpperCase())
                    .replace("projectName", this.projectName)
                    .replace("SubscriptionName", this.subscriptionName);
    }

    onPlanChange(plan: "Monthly" | "Yearly"): void {
        this.selectedPlan = plan;
        if (!this.isSubscribed) {
            this.setIframe(this.getCheckoutSrc());
        }
    }

    private setupMessageListener(): void {
        window.addEventListener("message", (event) => {
            // Verify origin for security
            if (event.data == "cb.loaded") {
                this.loaded = true;
                this.iframeDisplay = iframeDisplayVisible;
                setTimeout(() => {
                    this.iframeHeight = defaultIframeHeight;
                    this.iframeWidth = defaultIframeWidth;
                }, 500);

            }
        });
    }
}
