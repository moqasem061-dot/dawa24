/*
 * The advertisement wizard's behaviour.
 *
 * It used to live in an x-data attribute roughly eighty lines long, together
 * with the supplier's entire in-stock inventory serialised as JSON. Two
 * consequences, both reported as bugs: the page carried the whole catalogue on
 * every render, and the picker's filtering ran in the browser against that
 * array with no keyboard support and a dropdown that closed before its own
 * click landed.
 *
 * The picker is now components.Combobox (static/js/combobox.js), which fetches
 * on demand. What is left here is the step machine, and it is small enough to
 * read in one go.
 */
(function () {
	'use strict';

	function dawaAdsWizard(config) {
		return {
			isOpen: false,
			step: 1,
			totalSteps: config.totalSteps || 4,
			stepNames: config.stepNames || [],
			placement: config.placement || 'home_hero',
			placementLabels: config.placementLabels || {},
			totalCredits: config.totalCredits || 0,
			creditCost: config.creditCost || 2,

			clickTargetType: 'product',
			durationDays: 30,
			titleAr: '',
			titleEn: '',
			adTextAr: '',
			mediaType: 'image',
			mediaPreview: '',

			// selected mirrors the combobox's choice. The component dispatches
			// combobox-change on selection and on clearing, so the wizard never
			// reads the input's value directly.
			selected: null,

			init() {
				this.$el.addEventListener('combobox-change', (e) => {
					if (!e.detail || e.detail.name !== 'click_target_id') return;
					this.selected = e.detail.value
						? { id: e.detail.value, label: e.detail.item ? e.detail.item.label : '', item: e.detail.item }
						: null;
				});
			},

			open() {
				this.isOpen = true;
				this.step = 1;
			},

			close() {
				this.isOpen = false;
			},

			clearProduct() {
				this.selected = null;
				// Give the component back its own empty state.
				const hidden = this.$el.querySelector('input[name="click_target_id"]');
				if (hidden) hidden.value = '';
				if (window.dawaComboboxRegistry && window.dawaComboboxRegistry['click_target_id']) {
					window.dawaComboboxRegistry['click_target_id'].clear(false);
				}
			},

			onFileChange(event) {
				const file = event.target.files && event.target.files[0];
				if (!file) {
					this.mediaPreview = '';
					return;
				}
				this.mediaType = file.type && file.type.indexOf('video/') === 0 ? 'video' : 'image';
				const reader = new FileReader();
				reader.onload = (e) => { this.mediaPreview = e.target.result; };
				reader.readAsDataURL(file);
			},

			/* canAdvance and canSubmit are a courtesy, not a control. The
			 * handler validates every one of these again: a disabled button
			 * stops a person, not a crafted POST, and until now nothing on the
			 * server checked the product, the placement, the duration or the
			 * credits. */
			canAdvance() {
				if (this.step === 1) return !!this.selected;
				if (this.step === 3) return this.titleAr.trim().length > 0;
				return true;
			},

			canSubmit() {
				return !!this.selected
					&& this.titleAr.trim().length > 0
					&& this.totalCredits >= this.creditCost;
			},

			nextStep() {
				if (!this.canAdvance()) return;
				if (this.step < this.totalSteps) this.step++;
			},

			prevStep() {
				if (this.step > 1) this.step--;
			}
		};
	}

	window.dawaAdsWizard = dawaAdsWizard;
})();
