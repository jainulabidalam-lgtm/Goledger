document.addEventListener('DOMContentLoaded', () => {
    const state = {
        accounts: [],
        journals: [],
        displayCurrency: localStorage.getItem('goledger-display-currency') || 'INR',
        rates: { INR: 1 },
        rateDate: null,
        rateUpdatedAt: null,
    };

    const currencyCodes = ['INR', 'USD', 'EUR'];
    const currencyNames = { INR: 'Indian Rupee', USD: 'US Dollar', EUR: 'Euro' };
    const currencyLocales = { INR: 'en-IN', USD: 'en-US', EUR: 'de-DE' };
    let ratesPromise = null;

    const fetchExchangeRates = async (force = false) => {
        if (ratesPromise && !force) return ratesPromise;
        ratesPromise = (async () => {
            const status = document.getElementById('fx-rate-status');
            if (status) status.textContent = 'Refreshing exchange rates...';
            try {
                const response = await fetch('https://open.er-api.com/v6/latest/INR', {
                    cache: 'no-store',
                    headers: { 'Accept': 'application/json' }
                });
                if (!response.ok) throw new Error(`Rate service returned HTTP ${response.status}`);
                const payload = await response.json();
                if (payload.result !== 'success' || !payload.rates ||
                    !currencyCodes.every(code => Number.isFinite(payload.rates[code]) && payload.rates[code] > 0)) {
                    throw new Error('Rate service returned incomplete currency data.');
                }
                state.rates = { INR: 1, USD: payload.rates.USD, EUR: payload.rates.EUR };
                state.rateDate = payload.time_last_update_utc || payload.time_last_update_unix || null;
                state.rateUpdatedAt = new Date();
                if (status) status.textContent = 'Live rates loaded';
                const detail = document.getElementById('fx-rate-detail');
                if (detail) detail.textContent = `Rates based on INR. Last checked: ${state.rateUpdatedAt.toLocaleTimeString()}. Provider update: ${state.rateDate || 'not supplied'}.`;
                return true;
            } catch (error) {
                if (status) status.textContent = 'Exchange rates unavailable';
                const detail = document.getElementById('fx-rate-detail');
                if (detail) detail.textContent = 'Could not fetch live rates. No estimated conversion is shown; retry when online.';
                console.warn('GoLedger exchange-rate request failed:', error);
                return false;
            } finally {
                ratesPromise = null;
            }
        })();
        return ratesPromise;
    };

    const convertMajorAmount = (amount, fromCurrency, toCurrency) => {
        if (fromCurrency === toCurrency) return { amount, converted: true };
        const fromRate = state.rates[fromCurrency];
        const toRate = state.rates[toCurrency];
        if (!(fromRate > 0) || !(toRate > 0)) return { amount, converted: false };
        return { amount: amount / fromRate * toRate, converted: true };
    };

    const formatCurrency = (minorAmount, sourceCurrency = 'INR') => {
        const source = currencyCodes.includes(sourceCurrency) ? sourceCurrency : 'INR';
        const amount = Number(minorAmount || 0) / 100;
        const converted = convertMajorAmount(amount, source, state.displayCurrency);
        const currency = converted.converted ? state.displayCurrency : source;
        return new Intl.NumberFormat(currencyLocales[currency] || 'en-US', {
            style: 'currency', currency, minimumFractionDigits: 2, maximumFractionDigits: 2
        }).format(converted.amount);
    };

    const updateConverter = () => {
        const amountEl = document.getElementById('fx-amount');
        const fromEl = document.getElementById('fx-from');
        const toEl = document.getElementById('fx-to');
        const resultEl = document.getElementById('fx-result');
        if (!amountEl || !fromEl || !toEl || !resultEl) return;
        const amount = Number(amountEl.value);
        if (!Number.isFinite(amount) || amount < 0) {
            resultEl.textContent = 'Enter a valid amount';
            return;
        }
        const from = fromEl.value;
        const to = toEl.value;
        const converted = convertMajorAmount(amount, from, to);
        if (!converted.converted) {
            resultEl.textContent = from === to
                ? new Intl.NumberFormat(currencyLocales[to], { style: 'currency', currency: to }).format(amount)
                : 'Rate unavailable';
            return;
        }
        resultEl.textContent = new Intl.NumberFormat(currencyLocales[to], {
            style: 'currency', currency: to, minimumFractionDigits: 2, maximumFractionDigits: 2
        }).format(converted.amount);
    };

    const refreshDisplayedViews = async () => {
        const active = document.querySelector('.nav-item.active');
        const route = active ? active.dataset.route : 'dashboard';
        await navigate(route || 'dashboard');
    };

    const reportingCurrencySelect = document.getElementById('reporting-currency');
    if (reportingCurrencySelect) {
        reportingCurrencySelect.value = currencyCodes.includes(state.displayCurrency) ? state.displayCurrency : 'INR';
        state.displayCurrency = reportingCurrencySelect.value;
        reportingCurrencySelect.addEventListener('change', async () => {
            state.displayCurrency = reportingCurrencySelect.value;
            localStorage.setItem('goledger-display-currency', state.displayCurrency);
            await fetchExchangeRates();
            updateConverter();
            await refreshDisplayedViews();
        });
    }

    ['fx-amount', 'fx-from', 'fx-to'].forEach(id => {
        const el = document.getElementById(id);
        if (el) el.addEventListener('input', updateConverter);
        if (el && el.tagName === 'SELECT') el.addEventListener('change', updateConverter);
    });
    const refreshRatesButton = document.getElementById('fx-refresh');
    if (refreshRatesButton) refreshRatesButton.addEventListener('click', async () => {
        await fetchExchangeRates(true);
        updateConverter();
        await refreshDisplayedViews();
    });

    const views = document.querySelectorAll('.view');
    const navItems = document.querySelectorAll('.nav-item');
    const pageTitle = document.getElementById('page-title');
    const loadingIndicator = document.getElementById('loading-indicator');
    const errorContainer = document.getElementById('error-container');

    const showLoading = () => loadingIndicator.classList.remove('hidden');
    const hideLoading = () => loadingIndicator.classList.add('hidden');
    
    const showError = (msg) => {
        console.error("Dashboard Error:", msg);
        errorContainer.textContent = msg;
        errorContainer.classList.remove('hidden');
        setTimeout(() => errorContainer.classList.add('hidden'), 8000);
    };

    const apiCall = async (url, options = {}) => {
        showLoading();
        try {
            // Ensure no-cache so we always fetch latest data from the DB
            const fetchOptions = { ...options, cache: 'no-store' };
            const response = await fetch(url, fetchOptions);
            
            // Check if response has content
            const text = await response.text();
            let data = null;
            if (text) {
                try {
                    data = JSON.parse(text);
                } catch (e) {
                    throw new Error('Invalid JSON from server: ' + text.substring(0, 50));
                }
            }

            if (!response.ok) {
                throw new Error((data && data.error) ? data.error : `HTTP ${response.status} ${response.statusText}`);
            }
            return data;
        } catch (error) {
            showError(error.message);
            throw error;
        } finally {
            hideLoading();
        }
    };



    const navigate = async (route) => {
        views.forEach(v => v.classList.remove('active'));
        navItems.forEach(n => n.classList.remove('active'));
        
        const viewEl = document.getElementById(`view-${route}`);
        const navEl = document.querySelector(`[data-route="${route}"]`);
        
        if (viewEl) viewEl.classList.add('active');
        if (navEl) navEl.classList.add('active');

        try {
            switch(route) {
                case 'dashboard':
                    pageTitle.textContent = 'Dashboard';
                    await loadDashboard();
                    break;
                case 'accounts':
                    pageTitle.textContent = 'Chart of Accounts';
                    await loadAccounts();
                    break;
                case 'journals':
                    pageTitle.textContent = 'Journal Entries';
                    await loadJournalsView();
                    break;
                case 'ledger':
                    pageTitle.textContent = 'Account Ledger';
                    await loadLedgerView();
                    break;
            }
        } catch (err) {
            showError("Failed to load view: " + err.message);
        }
    };

    navItems.forEach(item => {
        item.addEventListener('click', (e) => {
            e.preventDefault();
            navigate(e.target.dataset.route);
        });
    });

    const loadDashboard = async () => {
        const accs = await apiCall('/api/accounts');
        const jnls = await apiCall('/api/journal-entries');
        
        state.accounts = accs || [];
        state.journals = jnls || [];

        document.getElementById('dash-total-accounts').textContent = state.accounts.length;
        document.getElementById('dash-total-journals').textContent = state.journals.length;

        const jBody = document.querySelector('#dash-journals-table tbody');
        if (state.journals.length > 0) {
            jBody.innerHTML = state.journals.slice(0, 5).map(j => `
                <tr>
                    <td>${j.date || ''}</td>
                    <td>${j.description || ''}</td>
                    <td><span style="color:var(--accent); font-weight:600;">${j.status || ''}</span></td>
                </tr>
            `).join('');
        } else {
            jBody.innerHTML = '<tr><td colspan="3">No journal entries found.</td></tr>';
        }

        const balancesBody = document.querySelector('#dash-balances-table tbody');
        balancesBody.innerHTML = '<tr><td colspan="3">Loading balances...</td></tr>';
        
        const accountsToLoad = state.accounts.slice(0, 10);
        if (accountsToLoad.length > 0) {
            try {
                const ledgers = await Promise.all(accountsToLoad.map(a => apiCall(`/api/accounts/${a.id}/ledger`)));
                balancesBody.innerHTML = ledgers.map(l => `
                    <tr>
                        <td>${l.account.code} - ${l.account.name}</td>
                        <td>${l.account.type}</td>
                        <td style="font-weight:600;">${formatCurrency(l.balance || 0, l.account.currency)}</td>
                    </tr>
                `).join('');
            } catch (err) {
                console.error(err);
                balancesBody.innerHTML = '<tr><td colspan="3">Error loading balances.</td></tr>';
            }
        } else {
            balancesBody.innerHTML = '<tr><td colspan="3">No accounts found.</td></tr>';
        }
    };

    const loadAccounts = async () => {
        const accs = await apiCall('/api/accounts');
        state.accounts = accs || [];
        const body = document.querySelector('#accounts-table tbody');
        if (state.accounts.length > 0) {
            body.innerHTML = state.accounts.map(a => `
                <tr>
                    <td>${a.code || ''}</td>
                    <td>${a.name || ''}</td>
                    <td>${a.type || ''}</td>
                    <td>${a.currency || ''}</td>
                    <td>${a.status || ''}</td>
                </tr>
            `).join('');
        } else {
            body.innerHTML = '<tr><td colspan="5">No accounts found.</td></tr>';
        }
    };

    const createAccountForm = document.getElementById('create-account-form');
    if (createAccountForm) {
        createAccountForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const payload = {
                code: document.getElementById('acc-code').value,
                name: document.getElementById('acc-name').value,
                type: document.getElementById('acc-type').value,
                currency: document.getElementById('acc-currency').value
            };

            try {
                await apiCall('/api/accounts', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(payload)
                });
                e.target.reset();
                await loadAccounts();
            } catch (err) {
                // error is already shown by apiCall
            }
        });
    }

    const loadJournalsView = async () => {
        const jnls = await apiCall('/api/journal-entries');
        const accs = await apiCall('/api/accounts');
        state.journals = jnls || [];
        state.accounts = accs || [];
        
        const body = document.querySelector('#journals-table tbody');
        if (state.journals.length > 0) {
            body.innerHTML = state.journals.map(j => `
                <tr>
                    <td>#${j.id || ''}</td>
                    <td>${j.date || ''}</td>
                    <td>${j.description || ''}</td>
                    <td><span style="color:var(--accent); font-weight:600;">${j.status || ''}</span></td>
                </tr>
            `).join('');
        } else {
            body.innerHTML = '<tr><td colspan="4">No journal entries found.</td></tr>';
        }

        const linesContainer = document.getElementById('journal-lines');
        if (linesContainer && linesContainer.children.length === 0) {
            addJournalLine();
            addJournalLine();
        }
    };

    const addJournalLine = () => {
        const container = document.getElementById('journal-lines');
        if (!container) return;
        
        const line = document.createElement('div');
        line.className = 'journal-line';
        
        const options = state.accounts.map(a => `<option value="${a.id}">${a.code} - ${a.name}</option>`).join('');
        
        line.innerHTML = `
            <div>
                <label>Account</label>
                <select class="j-line-account" required>
                    <option value="">-- Select Account --</option>
                    ${options}
                </select>
            </div>
            <div>
                <label>Debit</label>
                <input type="number" class="j-line-debit" min="0" step="0.01" value="0.00">
            </div>
            <div>
                <label>Credit</label>
                <input type="number" class="j-line-credit" min="0" step="0.01" value="0.00">
            </div>
            <div>
                <span class="text-danger remove-line" title="Remove Line" style="cursor:pointer;font-weight:bold;">✕</span>
            </div>
        `;

        line.querySelector('.remove-line').addEventListener('click', () => {
            if (document.querySelectorAll('.journal-line').length > 2) {
                line.remove();
                updateTotals();
            } else {
                showError("Minimum 2 lines required for a journal entry.");
            }
        });

        line.querySelectorAll('input').forEach(i => i.addEventListener('input', updateTotals));
        container.appendChild(line);
    };

    const btnAddLine = document.getElementById('btn-add-line');
    if (btnAddLine) {
        btnAddLine.addEventListener('click', addJournalLine);
    }

    const updateTotals = () => {
        let totalDebit = 0;
        let totalCredit = 0;
        document.querySelectorAll('.journal-line').forEach(line => {
            const debit = parseFloat(line.querySelector('.j-line-debit').value) || 0;
            const credit = parseFloat(line.querySelector('.j-line-credit').value) || 0;
            totalDebit += debit;
            totalCredit += credit;
        });
        
        const debitEl = document.getElementById('total-debit');
        const creditEl = document.getElementById('total-credit');
        if (debitEl) debitEl.textContent = totalDebit.toFixed(2);
        if (creditEl) creditEl.textContent = totalCredit.toFixed(2);
    };

    const createJournalForm = document.getElementById('create-journal-form');
    if (createJournalForm) {
        createJournalForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            
            const lines = Array.from(document.querySelectorAll('.journal-line')).map(line => {
                return {
                    account_id: parseInt(line.querySelector('.j-line-account').value),
                    debit_amount: Math.round((parseFloat(line.querySelector('.j-line-debit').value) || 0) * 100),
                    credit_amount: Math.round((parseFloat(line.querySelector('.j-line-credit').value) || 0) * 100)
                };
            });

            const payload = {
                date: document.getElementById('j-date').value,
                description: document.getElementById('j-desc').value,
                lines: lines
            };

            try {
                await apiCall('/api/journal-entries', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(payload)
                });
                e.target.reset();
                document.getElementById('journal-lines').innerHTML = '';
                addJournalLine();
                addJournalLine();
                updateTotals();
                await loadJournalsView();
            } catch (err) {}
        });
    }

    const loadLedgerView = async () => {
        const accs = await apiCall('/api/accounts');
        state.accounts = accs || [];
        const select = document.getElementById('ledger-account-select');
        if (select) {
            select.innerHTML = '<option value="">-- Select an Account --</option>' + 
                state.accounts.map(a => `<option value="${a.id}">${a.code} - ${a.name}</option>`).join('');
        }
    };

    const ledgerSelect = document.getElementById('ledger-account-select');
    if (ledgerSelect) {
        ledgerSelect.addEventListener('change', async (e) => {
            const id = e.target.value;
            const resultsEl = document.getElementById('ledger-results');
            if (!id) {
                resultsEl.classList.add('hidden');
                return;
            }

            try {
                const data = await apiCall(`/api/accounts/${id}/ledger`);
                if (!data || !data.account) throw new Error("Invalid ledger data");
                
                document.getElementById('ledger-title').textContent = `${data.account.code} - ${data.account.name}`;
                document.getElementById('ledger-subtitle').textContent = `Type: ${data.account.type} | Currency: ${data.account.currency}`;
                document.getElementById('ledger-balance-amount').textContent = formatCurrency(data.balance || 0);
                
                const body = document.querySelector('#ledger-table tbody');
                if (data.entries && data.entries.length > 0) {
                    body.innerHTML = data.entries.map(e => `
                        <tr>
                            <td>${e.date || ''}</td>
                            <td>${e.description || ''}</td>
                            <td>${e.debit_amount > 0 ? formatCurrency(e.debit_amount) : ''}</td>
                            <td>${e.credit_amount > 0 ? formatCurrency(e.credit_amount) : ''}</td>
                            <td style="font-weight:600;">${formatCurrency(e.running_balance || 0, data.account.currency)}</td>
                        </tr>
                    `).join('');
                } else {
                    body.innerHTML = '<tr><td colspan="5">No transactions found.</td></tr>';
                }
                
                resultsEl.classList.remove('hidden');
            } catch (err) {
                resultsEl.classList.add('hidden');
            }
        });
    }

    const jDate = document.getElementById('j-date');
    if (jDate) jDate.valueAsDate = new Date();

    fetchExchangeRates().then(() => {
        updateConverter();
        navigate('dashboard');
    });
});

