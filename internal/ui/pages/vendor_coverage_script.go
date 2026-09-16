package pages

func vendorCoverageAlpineData() string {
	return `{
		filterBranch: (new URLSearchParams(window.location.search).get('branch_id') || 'all'),
		filterDay: (new URLSearchParams(window.location.search).get('day') || 'all'),
		filterGov: 'all',
		searchQuery: '',
		currentPage: 1,
		pageSize: 15,
		allCoverages: [],
		selectedDays: [6, 0, 1, 2, 3, 4, 5],
		selectedGovId: '',
		selectedCities: [],
		allCitiesInGov: false,
		citySearch: '',
		coverageFrom: '09:00',
		coverageTo: '17:00',
		allCitiesList: [],
		cityConfigs: {},
		editModalOpen: false,
		editCov: {
			id: 0,
			branch_id: '',
			governorate_id: '',
			city_id: '',
			day_of_week: 0,
			coverage_from: '',
			coverage_to: '',
			address: '',
			latitude: '',
			longitude: '',
			is_active: true
		},
		init() {
			const el = document.getElementById('cities-dataset');
			if (el) {
				try {
					const raw = el.getAttribute('data-cities') || '[]';
					this.allCitiesList = JSON.parse(raw);
				} catch(e) {
					console.error('Failed to parse cities JSON:', e);
					this.allCitiesList = [];
				}
			}
			const elCov = document.getElementById('coverages-dataset');
			if (elCov) {
				try {
					const rawCov = elCov.getAttribute('data-coverages') || '[]';
					this.allCoverages = JSON.parse(rawCov);
				} catch(e) {
					console.error('Failed to parse coverages JSON:', e);
					this.allCoverages = [];
				}
			}
		},
		toggleDay(d) {
			const idx = this.selectedDays.indexOf(d);
			if (idx > -1) {
				this.selectedDays.splice(idx, 1);
			} else {
				this.selectedDays.push(d);
			}
		},
		selectAllDays() {
			this.selectedDays = [0, 1, 2, 3, 4, 5, 6];
		},
		clearDays() {
			this.selectedDays = [];
		},
		onGovChange() {
			this.selectedCities = [];
			this.allCitiesInGov = false;
			this.citySearch = '';
			if (this.selectedGovId) {
				const gId = parseInt(this.selectedGovId, 10);
				const existing = this.allCitiesList.filter(c => c.gov_id === gId);
				if (existing.length === 0) {
					fetch('/vendor/coverage/governorates/' + gId + '/cities')
						.then(r => r.json())
						.then(items => {
							if (Array.isArray(items)) {
								items.forEach(item => {
									if (!this.allCitiesList.some(x => x.id === item.id)) {
										this.allCitiesList.push(item);
									}
								});
							}
						})
						.catch(e => console.error('Failed to load cities via API:', e));
				}
			}
		},
		get filteredCities() {
			if (!this.selectedGovId) return [];
			const gId = parseInt(this.selectedGovId, 10);
			return this.allCitiesList.filter(c => {
				const matchGov = (c.gov_id === gId) || (String(c.gov_id) === String(this.selectedGovId));
				if (!matchGov) return false;
				if (!this.citySearch) return true;
				const q = this.citySearch.toLowerCase();
				return (c.name_ar && c.name_ar.toLowerCase().includes(q)) || (c.name_en && c.name_en.toLowerCase().includes(q));
			});
		},
		initCityConfig(idStr) {
			if (!this.cityConfigs[idStr]) {
				this.cityConfigs[idStr] = {
					from: this.coverageFrom || '09:00',
					to: this.coverageTo || '17:00'
				};
			}
			return this.cityConfigs[idStr];
		},
		applyDefaultsToAllCities() {
			this.selectedCities.forEach(idStr => {
				this.cityConfigs[idStr] = {
					from: this.coverageFrom || '09:00',
					to: this.coverageTo || '17:00'
				};
			});
		},
		getSelectedCityDetails() {
			return this.allCitiesList.filter(c => this.selectedCities.includes(String(c.id)));
		},
		toggleCity(cId) {
			const idStr = String(cId);
			const idx = this.selectedCities.indexOf(idStr);
			if (idx > -1) {
				this.selectedCities.splice(idx, 1);
				this.allCitiesInGov = false;
			} else {
				this.selectedCities.push(idStr);
				this.initCityConfig(idStr);
			}
		},
		toggleSelectAllCities() {
			const gId = parseInt(this.selectedGovId, 10);
			const available = this.allCitiesList.filter(c => c.gov_id === gId || String(c.gov_id) === String(this.selectedGovId));
			if (this.allCitiesInGov || this.selectedCities.length === available.length) {
				this.selectedCities = [];
				this.allCitiesInGov = false;
			} else {
				this.selectedCities = available.map(c => String(c.id));
				this.allCitiesInGov = true;
				this.selectedCities.forEach(idStr => this.initCityConfig(idStr));
			}
		},
		totalCoverageKm() {
			return this.getSelectedCityDetails()
				.reduce((sum, c) => sum + ((c.radius_m || 0) / 1000), 0)
				.toFixed(1);
		},
		setTimePreset(from, to) {
			this.coverageFrom = from;
			this.coverageTo = to;
			this.applyDefaultsToAllCities();
		},
		clock12(hhmm) {
			if (!hhmm) return '';
			const p = String(hhmm).split(':');
			if (p.length < 2) return hhmm;
			let h = parseInt(p[0], 10);
			const m = parseInt(p[1], 10);
			if (isNaN(h) || isNaN(m)) return hhmm;
			const suffix = h >= 12 ? 'م' : 'ص';
			h = h % 12; if (h === 0) h = 12;
			return h + ':' + String(m).padStart(2, '0') + ' ' + suffix;
		},
		coverageWindowLabel() {
			if (!this.coverageFrom && !this.coverageTo) return 'طوال اليوم (24 ساعة)';
			if (this.coverageFrom && this.coverageTo) return this.clock12(this.coverageFrom) + ' – ' + this.clock12(this.coverageTo);
			return this.clock12(this.coverageFrom || this.coverageTo);
		},
		get filteredCoverages() {
			let list = this.allCoverages || [];
			if (this.filterBranch && this.filterBranch !== 'all') {
				const bId = String(this.filterBranch);
				list = list.filter(c => String(c.branch_id) === bId);
			}
			if (this.filterDay && this.filterDay !== 'all') {
				const d = parseInt(this.filterDay, 10);
				list = list.filter(c => c.day_of_week === d);
			}
			if (this.searchQuery && this.searchQuery.trim() !== '') {
				const q = this.searchQuery.trim().toLowerCase();
				list = list.filter(c => {
					return (c.governorate_name && c.governorate_name.toLowerCase().includes(q)) ||
						(c.city_name && c.city_name.toLowerCase().includes(q)) ||
						(c.branch_name && c.branch_name.toLowerCase().includes(q)) ||
						(c.address && c.address.toLowerCase().includes(q));
				});
			}
			return list;
		},
		get totalFilteredCount() {
			return this.filteredCoverages.length;
		},
		get totalPages() {
			if (this.pageSize <= 0 || this.pageSize === -1) return 1;
			return Math.max(1, Math.ceil(this.totalFilteredCount / this.pageSize));
		},
		get paginatedCoverages() {
			if (this.pageSize <= 0 || this.pageSize === -1) {
				return this.filteredCoverages;
			}
			if (this.currentPage > this.totalPages) {
				this.currentPage = this.totalPages;
			}
			const start = (this.currentPage - 1) * this.pageSize;
			return this.filteredCoverages.slice(start, start + this.pageSize);
		},
		get startIndex() {
			if (this.totalFilteredCount === 0) return 0;
			if (this.pageSize <= 0 || this.pageSize === -1) return 1;
			return (this.currentPage - 1) * this.pageSize + 1;
		},
		get endIndex() {
			if (this.pageSize <= 0 || this.pageSize === -1) return this.totalFilteredCount;
			return Math.min(this.currentPage * this.pageSize, this.totalFilteredCount);
		},
		nextPage() {
			if (this.currentPage < this.totalPages) {
				this.currentPage++;
			}
		},
		prevPage() {
			if (this.currentPage > 1) {
				this.currentPage--;
			}
		},
		goToPage(p) {
			if (p >= 1 && p <= this.totalPages) {
				this.currentPage = p;
			}
		},
		updateQueryParams() {
			const params = new URLSearchParams(window.location.search);
			if (this.filterBranch && this.filterBranch !== 'all') {
				params.set('branch_id', this.filterBranch);
			} else {
				params.delete('branch_id');
			}
			if (this.filterDay && this.filterDay !== 'all') {
				params.set('day', this.filterDay);
			} else {
				params.delete('day');
			}
			const newQuery = params.toString();
			const newUrl = window.location.pathname + (newQuery ? '?' + newQuery : '');
			window.history.replaceState({}, '', newUrl);
		},
		openEdit(c) {
			this.editCov = {
				id: c.id,
				branch_id: String(c.branch_id),
				governorate_id: String(c.governorate_id || ''),
				city_id: String(c.city_id || ''),
				day_of_week: c.day_of_week,
				distance_meters: c.distance_meters,
				coverage_from: c.coverage_from || '',
				coverage_to: c.coverage_to || '',
				address: c.address || '',
				latitude: c.latitude != null ? String(c.latitude) : '',
				longitude: c.longitude != null ? String(c.longitude) : '',
				is_active: !!c.is_active
			};
			const m = document.getElementById('edit-coverage-modal');
			if (m && typeof m.showModal === 'function') { try { m.showModal(); } catch(_) { m.setAttribute('open', ''); } }
		},
		closeEdit() {
			const m = document.getElementById('edit-coverage-modal');
			if (m && typeof m.close === 'function') { try { m.close(); } catch(_) { m.removeAttribute('open'); } }
		},
		openDeleteAllModal() {
			const m = document.getElementById('delete-all-coverage-modal');
			if (m && typeof m.showModal === 'function') { try { m.showModal(); } catch(_) { m.setAttribute('open', ''); } }
		},
		closeDeleteAllModal() {
			const m = document.getElementById('delete-all-coverage-modal');
			if (m && typeof m.close === 'function') { try { m.close(); } catch(_) { m.removeAttribute('open'); } }
		}
	}`
}