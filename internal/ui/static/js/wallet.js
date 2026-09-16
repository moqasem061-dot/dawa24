/*
 * The wallet page Alpine component (dawaWallet).
 *
 * Handles:
 * 1. Payment methods dialog (add/edit)
 * 2. Deposit modal with dynamic platform payment method details card, and
 *    matching sender payment methods filtered by the same provider type.
 * 3. Withdrawal modal with balance checks and destination syncing.
 */
(function () {
	'use strict';

	var EMPTY = {
		account_holder: '',
		bank_name: '',
		iban: '',
		account_number: '',
		instapay_handle: '',
		wallet_provider: '',
		wallet_phone: '',
		card_brand: '',
		card_last4: '',
		is_default: false
	};

	function normalizeType(t) {
		if (!t) return '';
		t = String(t).toLowerCase().trim();
		if (t === 'vodafone_cash' || t === 'wallet' || t === 'e_wallet' || t === 'mobile_wallet') return 'wallet';
		if (t === 'bank' || t === 'bank_transfer') return 'bank';
		if (t === 'instapay') return 'instapay';
		if (t === 'card' || t === 'credit_card' || t === 'debit_card') return 'card';
		return t;
	}

	function dawaWallet(config) {
		config = config || {};
		var base = (config && config.base) || '/customer/wallet';
		var platformMethods = (config && config.platformMethods) || [];
		var userMethods = (config && config.userMethods) || [];

		var firstPlat = platformMethods.length > 0 ? platformMethods[0] : null;
		var initPlatID = firstPlat ? firstPlat.id : '';
		var initPlatType = firstPlat ? normalizeType(firstPlat.provider_type) : 'bank';

		return {
			activeSection: 'transactions',
			isDepositModalOpen: false,
			isWithdrawModalOpen: false,
			isAddPaymentModalOpen: false,

			base: base,
			paymentType: 'bank',
			paymentEditID: 0,
			paymentForm: Object.assign({}, EMPTY),

			platformMethods: platformMethods,
			userMethods: userMethods,

			selectedPlatformId: initPlatID,
			selectedPlatformType: initPlatType,
			selectedSenderPMID: 'manual',
			senderAccount: '',

			withdrawPayoutType: 'bank',
			withdrawUserMethodId: '0',
			withdrawDestinationDetails: '',

			init: function () {
				this.syncPlatformDetails();
				this.updateSenderDropdown();
				this.syncWithdrawDetails();
			},

			get selectedPlatform() {
				var self = this;
				return this.platformMethods.find(function (p) {
					return p.id === self.selectedPlatformId;
				}) || null;
			},

			get filteredSenderMethods() {
				var norm = normalizeType(this.selectedPlatformType);
				return this.userMethods.filter(function (m) {
					return normalizeType(m.provider) === norm;
				});
			},

			syncPlatformDetails: function () {
				var p = this.selectedPlatform;
				if (p) {
					this.selectedPlatformType = normalizeType(p.provider_type);
				}
			},

			onPlatformChange: function () {
				this.syncPlatformDetails();
				this.updateSenderDropdown();
			},

			updateSenderDropdown: function () {
				var sel = (this.$refs && this.$refs.senderSelect) || document.getElementById('deposit-sender-select');
				if (!sel) return;

				var currentVal = this.selectedSenderPMID;
				sel.innerHTML = '';

				var matches = this.filteredSenderMethods;

				if (matches.length > 0) {
					var group = document.createElement('optgroup');
					group.label = 'حساباتك المسجلة المطابقة (' + matches.length + ')';
					matches.forEach(function (m) {
						var opt = document.createElement('option');
						opt.value = String(m.id);
						opt.textContent = m.identifier + ' (حسابك المسجل)';
						group.appendChild(opt);
					});
					sel.appendChild(group);
				}

				var manualOpt = document.createElement('option');
				manualOpt.value = 'manual';
				manualOpt.textContent = matches.length === 0
					? '-- لا توجد وسيلة مسجلة من هذا النوع (إدخال يدوي) --'
					: '-- إدخال رقم الحساب / المحفظة يدوياً --';
				sel.appendChild(manualOpt);

				var matchFound = matches.find(function (m) {
					return String(m.id) === String(currentVal);
				});

				if (matchFound) {
					this.selectedSenderPMID = String(matchFound.id);
					this.senderAccount = matchFound.identifier;
					sel.value = String(matchFound.id);
				} else if (matches.length > 0) {
					this.selectedSenderPMID = String(matches[0].id);
					this.senderAccount = matches[0].identifier;
					sel.value = String(matches[0].id);
				} else {
					this.selectedSenderPMID = 'manual';
					this.senderAccount = '';
					sel.value = 'manual';
				}
			},

			onSenderMethodChange: function () {
				var sel = (this.$refs && this.$refs.senderSelect) || document.getElementById('deposit-sender-select');
				if (sel) {
					this.selectedSenderPMID = sel.value;
				}
				if (this.selectedSenderPMID === 'manual') {
					this.senderAccount = '';
				} else {
					var self = this;
					var m = this.userMethods.find(function (x) {
						return String(x.id) === String(self.selectedSenderPMID);
					});
					if (m) {
						this.senderAccount = m.identifier;
					}
				}
			},

			syncWithdrawDetails: function () {
				if (this.withdrawUserMethodId === 'manual' || this.withdrawUserMethodId === '0') {
					this.withdrawDestinationDetails = '';
				} else {
					var self = this;
					var m = this.userMethods.find(function (x) {
						return String(x.id) === String(self.withdrawUserMethodId);
					});
					if (m) {
						this.withdrawDestinationDetails = m.identifier;
						this.withdrawPayoutType = normalizeType(m.provider);
					}
				}
			},

			onWithdrawMethodChange: function () {
				this.syncWithdrawDetails();
			},

			openDepositModal: function () {
				this.isDepositModalOpen = true;
				var self = this;
				var syncFn = function () {
					self.onPlatformChange();
				};
				if (this.$nextTick) {
					this.$nextTick(syncFn);
				} else {
					setTimeout(syncFn, 50);
				}
			},

			openWithdrawModal: function () {
				this.isWithdrawModalOpen = true;
				if ((!this.withdrawUserMethodId || this.withdrawUserMethodId === '0' || this.withdrawUserMethodId === '') && this.userMethods && this.userMethods.length > 0) {
					var def = this.userMethods.find(function (m) { return m.is_default; }) || this.userMethods[0];
					this.withdrawUserMethodId = String(def.id);
				}
				var self = this;
				var syncFn = function () {
					self.onWithdrawMethodChange();
				};
				if (this.$nextTick) {
					this.$nextTick(syncFn);
				} else {
					setTimeout(syncFn, 50);
				}
			},

			openPaymentAdd: function () {
				this.paymentEditID = 0;
				this.paymentType = 'bank';
				this.paymentForm = Object.assign({}, EMPTY);
				this.isAddPaymentModalOpen = true;
			},

			openPaymentEdit: function (el) {
				var pm;
				try {
					pm = JSON.parse(el.dataset.paymentMethod || '{}');
				} catch (e) {
					return;
				}

				this.paymentEditID = pm.id || 0;
				this.paymentType = pm.type || 'bank';
				this.paymentForm = Object.assign({}, EMPTY, {
					account_holder: pm.account_holder || '',
					bank_name: pm.bank_name || '',
					iban: pm.iban || '',
					account_number: pm.account_number || '',
					instapay_handle: pm.instapay_handle || '',
					wallet_provider: pm.wallet_provider || '',
					wallet_phone: pm.wallet_phone || '',
					card_brand: pm.card_brand || '',
					card_last4: pm.card_last4 || '',
					is_default: !!pm.is_default
				});
				this.isAddPaymentModalOpen = true;
			},

			paymentFormAction: function () {
				if (this.paymentEditID) {
					return this.base + '/payment-methods/' + this.paymentEditID + '/edit';
				}
				return this.base + '/payment-methods';
			}
		};
	}

	window.dawaWallet = dawaWallet;

	if (window.Alpine) {
		Alpine.data('dawaWallet', dawaWallet);
	}
	document.addEventListener('alpine:init', function () {
		if (window.Alpine) {
			Alpine.data('dawaWallet', dawaWallet);
		}
	});
})();
