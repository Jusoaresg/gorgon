(function () {
    'use strict';

    var WS_URL = (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/api/v1/ws';

    var TRACKING_CLASSES = ['wanted', 'missing', 'skipped', 'snatched', 'downloaded'];

    var TRACKING_LABELS = {
        wanted: 'Wanted',
        missing: 'Missing',
        skipped: 'Skipped',
        snatched: 'Snatched',
        downloaded: 'Downloaded'
    };

    function removeTrackingClasses(el) {
        TRACKING_CLASSES.forEach(function (cls) {
            el.classList.remove(cls);
        });
    }

    function applyTracking(el, tracking) {
        removeTrackingClasses(el);
        if (TRACKING_CLASSES.indexOf(tracking) !== -1) {
            el.classList.add(tracking);
        }
    }

    function updateEpisode(episodeId, tracking, infoUrl) {
        var targets = document.querySelectorAll('[data-episode-id="' + episodeId + '"]');
        targets.forEach(function (target) {
            if (target.classList.contains('episode-card')) {
                applyTracking(target, tracking);

                var badge = target.querySelector('.status-badge');
                if (badge) {
                    applyTracking(badge, tracking);
                    badge.textContent = tracking;
                }

                var deleteBtn = target.querySelector('.btn-delete');
                if (deleteBtn) {
                    deleteBtn.style.display = tracking === 'downloaded' ? 'flex' : 'none';
                }

                var searchActions = target.querySelectorAll('.js-search-actions');
                searchActions.forEach(function (btn) {
                    btn.style.display = tracking === 'downloaded' ? 'none' : 'flex';
                });

                var shouldShowRelease = (tracking === 'downloaded' || tracking === 'snatched') && infoUrl;
                var releaseLink = target.querySelector('.js-release-link');
                if (shouldShowRelease) {
                    if (!releaseLink) {
                        var actionButtons = target.querySelector('.action-buttons');
                        if (actionButtons) {
                            releaseLink = document.createElement('a');
                            releaseLink.className = 'btn-icon-sm btn-release js-release-link';
                            releaseLink.target = '_blank';
                            releaseLink.title = 'View Release';
                            releaseLink.innerHTML = RELEASE_ICON;
                            actionButtons.appendChild(releaseLink);
                        }
                    }
                    if (releaseLink) {
                        releaseLink.href = infoUrl;
                        releaseLink.style.display = 'flex';
                    }
                } else if (releaseLink) {
                    releaseLink.style.display = 'none';
                }
            }

            if (target.classList.contains('cal-card-track')) {
                applyTracking(target, tracking);
                target.textContent = TRACKING_LABELS[tracking] || tracking || 'Unknown';
            }
        });
    }

    var TOAST_ICONS = {
        success: '<svg class="toast-icon" width="20" height="20" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>',
        error: '<svg class="toast-icon" width="20" height="20" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v4m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"></path></svg>',
        info: '<svg class="toast-icon" width="20" height="20" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>'
    };

    var CLOSE_ICON = '<svg class="toast-close" width="18" height="18" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>';

    var RELEASE_ICON = '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path></svg>';

    function toastContainer() {
        var container = document.getElementById('toast-container');
        if (!container) {
            container = document.createElement('div');
            container.id = 'toast-container';
            document.body.appendChild(container);
        }
        container.classList.add('toast-container');
        return container;
    }

    var MAX_VISIBLE_TOASTS = 3;

    function isSearchStartMessage(msg) {
        if (!msg) return false;
        var lower = msg.toLowerCase();
        return lower.indexOf('search started') !== -1 || lower.indexOf('searching') !== -1;
    }

    function isSearchResultMessage(msg) {
        if (!msg) return false;
        var lower = msg.toLowerCase();
        return lower.indexOf('snatched') !== -1 || lower.indexOf('no results') !== -1 || lower.indexOf('search failed') !== -1;
    }

    function dismissToast(toast) {
        if (!toast || toast.classList.contains('toast-removing')) return;
        if (toast._dismissTimer) {
            clearTimeout(toast._dismissTimer);
            toast._dismissTimer = null;
        }
        toast.classList.add('toast-removing');
        toast.addEventListener('animationend', function () {
            if (toast.parentNode) toast.remove();
        }, { once: true });
        setTimeout(function () {
            if (toast.parentNode) toast.remove();
        }, 250);
    }

    function scheduleToastDismiss(toast, duration) {
        if (toast._dismissTimer) clearTimeout(toast._dismissTimer);
        toast._dismissTimer = setTimeout(function () {
            dismissToast(toast);
        }, duration);
    }

    function displayToast(message, type) {
        if (!message) return;
        type = type || 'info';

        var container = toastContainer();

        // 1. If this is a search started message, dismiss any existing search started toast
        if (isSearchStartMessage(message)) {
            var existingSearch = container.querySelectorAll('.toast[data-is-search="true"]:not(.toast-removing)');
            existingSearch.forEach(function (t) {
                dismissToast(t);
            });
        }

        // 2. If this is a search result message, dismiss any lingering search started toast
        if (isSearchResultMessage(message)) {
            var activeSearchToasts = container.querySelectorAll('.toast[data-is-search="true"]:not(.toast-removing)');
            activeSearchToasts.forEach(function (t) {
                dismissToast(t);
            });
        }

        // 3. Deduplication: If identical message is already visible, increment count badge and refresh timer
        var visibleToasts = container.querySelectorAll('.toast:not(.toast-removing)');
        for (var i = 0; i < visibleToasts.length; i++) {
            var existing = visibleToasts[i];
            if (existing.dataset.rawMessage === message) {
                var count = parseInt(existing.dataset.count || '1', 10) + 1;
                existing.dataset.count = count;

                var badge = existing.querySelector('.toast-badge');
                if (!badge) {
                    badge = document.createElement('span');
                    badge.className = 'toast-badge';
                    var targetHeader = existing.querySelector('.toast-title') || existing;
                    targetHeader.appendChild(badge);
                }
                badge.textContent = count + 'x';

                var prog = existing.querySelector('.toast-progress');
                if (prog) {
                    prog.style.animation = 'none';
                    void prog.offsetWidth;
                    prog.style.animation = '';
                }

                existing.classList.remove('toast-pulse');
                void existing.offsetWidth; // trigger reflow for pulse animation
                existing.classList.add('toast-pulse');

                var repeatDuration = type === 'error' ? 5000 : 2800;
                scheduleToastDismiss(existing, repeatDuration);
                return;
            }
        }

        // 4. Enforce stack limit (max 3 visible toasts)
        var currentVisible = container.querySelectorAll('.toast:not(.toast-removing)');
        while (currentVisible.length >= MAX_VISIBLE_TOASTS) {
            dismissToast(currentVisible[0]);
            currentVisible = container.querySelectorAll('.toast:not(.toast-removing)');
        }

        // 5. Create new toast element
        var autoDuration = type === 'error' ? 5000 : (type === 'success' ? 2800 : 3200);

        var toast = document.createElement('div');
        toast.className = 'toast toast-' + type;
        toast.dataset.rawMessage = message;
        toast.style.setProperty('--toast-duration', autoDuration + 'ms');
        if (isSearchStartMessage(message)) {
            toast.dataset.isSearch = 'true';
        }

        var iconBox = document.createElement('div');
        iconBox.className = 'toast-icon-box';
        iconBox.innerHTML = TOAST_ICONS[type] || TOAST_ICONS.info;

        var content = document.createElement('div');
        content.className = 'toast-content';

        var titleEl = document.createElement('div');
        titleEl.className = 'toast-title';

        // Split " — " if present (e.g. "S01E03 Pilot — Snatched")
        var sepIdx = message.indexOf(' — ');
        if (sepIdx !== -1) {
            var mainTitle = message.substring(0, sepIdx);
            var statusPart = message.substring(sepIdx + 3);

            var titleText = document.createElement('span');
            titleText.textContent = mainTitle;
            titleEl.appendChild(titleText);

            var statusPill = document.createElement('span');
            var pillClass = 'toast-status-pill ';
            var statusLower = statusPart.toLowerCase();
            if (statusLower.indexOf('snatched') !== -1) {
                pillClass += 'snatched';
            } else if (statusLower.indexOf('no results') !== -1 || statusLower.indexOf('not aired') !== -1) {
                pillClass += 'no-results';
            } else {
                pillClass += 'error';
            }
            statusPill.className = pillClass;
            statusPill.textContent = statusPart;
            titleEl.appendChild(statusPill);
        } else {
            titleEl.textContent = message;
        }

        content.appendChild(titleEl);

        var close = document.createElement('button');
        close.type = 'button';
        close.className = 'toast-close';
        close.title = 'Dismiss';
        close.innerHTML = CLOSE_ICON;
        close.addEventListener('click', function (e) {
            e.stopPropagation();
            dismissToast(toast);
        });

        var progress = document.createElement('div');
        progress.className = 'toast-progress';

        toast.appendChild(iconBox);
        toast.appendChild(content);
        toast.appendChild(close);
        toast.appendChild(progress);

        // Click anywhere on toast to dismiss
        toast.addEventListener('click', function () {
            dismissToast(toast);
        });

        // Hover pause / resume
        toast.addEventListener('mouseenter', function () {
            if (toast._dismissTimer) clearTimeout(toast._dismissTimer);
        });
        toast.addEventListener('mouseleave', function () {
            scheduleToastDismiss(toast, 2000);
        });

        container.appendChild(toast);
        scheduleToastDismiss(toast, autoDuration);
    }

    function handleAliasAdded(event) {
        if (!event.detail.successful) return;

        var form = event.detail.elt;
        var input = form.querySelector('input[name="alias"]');
        var alias = (input && input.value || '').trim();

        var data = {};
        try {
            data = JSON.parse(event.detail.xhr.response).data || {};
        } catch (e) {}

        var aliasId = data.id;
        if (!alias || !aliasId) return;

        var manager = form.closest('.modal-alias-manager');
        var list = manager.querySelector('.alias-manage-list');
        if (!list) {
            var empty = manager.querySelector('.text-secondary');
            if (empty) empty.remove();
            list = document.createElement('ul');
            list.className = 'alias-manage-list';
            manager.insertBefore(list, form);
        }

        var li = document.createElement('li');
        li.className = 'alias-manage-item';

        var name = document.createElement('span');
        name.textContent = alias;

        var del = document.createElement('button');
        del.className = 'btn-icon-sm btn-delete';
        del.title = 'Delete alias';
        del.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-trash" viewBox="0 0 16 16"><path d="M5.5 5.5A.5.5 0 0 1 6 6v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5m2.5 0a.5.5 0 0 1 .5.5v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5m3 .5a.5.5 0 0 0-1 0v6a.5.5 0 0 0 1 0z"/><path d="M14.5 3a1 1 0 0 1-1 1H13v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V4h-.5a1 1 0 0 1-1-1V2a1 1 0 0 1 1-1H6a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1h3.5a1 1 0 0 1 1 1zM4.118 4 4 4.059V13a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1V4.059L11.882 4zM2.5 3h11V2h-11z"/></svg>';
        del.setAttribute('hx-delete', form.getAttribute('hx-post').replace(/\/alias$/, '/alias/' + aliasId));
        del.setAttribute('hx-swap', 'none');
        del.setAttribute('hx-confirm', 'Delete alias \'' + alias + '\'?');
        del.setAttribute('hx-on::after-request', 'showToast(event); if (event.detail.successful) this.closest(\'.alias-manage-item\').remove()');

        li.appendChild(name);
        li.appendChild(del);
        list.appendChild(li);
        htmx.process(li);

        if (input) input.value = '';
    }

    function showToast(event) {
        var message;
        var type = 'info';

        try {
            var response = JSON.parse(event.detail.xhr.response);
            var data = response.data;
            if (Array.isArray(data)) {
                data = data[0];
            }
            message = (data && data.toastMessage) || response.message || null;
        } catch (e) {
            message = null;
        }

        if (!message) {
            var status = event.detail.xhr.status;
            type = status >= 200 && status < 300 ? 'success' : 'error';
            message = type === 'success'
                ? 'Request successful'
                : status >= 500
                    ? 'Something went wrong on the server'
                    : 'Something went wrong';
        } else {
            type = event.detail.xhr.status >= 200 && event.detail.xhr.status < 300 ? 'success' : 'error';
        }

        displayToast(message, type);
    }

    function padEpisodeNumber(n) {
        return n < 10 ? '0' + n : '' + n;
    }

    function onSearchFinished(message) {
        var label = 'S' + message.season + 'E' + padEpisodeNumber(message.number);
        if (message.name) {
            label += ' ' + message.name;
        }
        switch (message.result) {
            case 'snatched':
                displayToast(label + ' — Snatched', 'success');
                break;
            case 'noResults':
                displayToast(label + ' — No results found', 'info');
                break;
            case 'notAired':
                displayToast(label + ' — Not aired yet', 'info');
                break;
            case 'error':
                displayToast(label + ' — Search failed', 'error');
                break;
        }
    }

    function connect() {
        var socket;
        try {
            socket = new WebSocket(WS_URL);
        } catch (e) {
            return;
        }

        socket.onmessage = function (event) {
            var message;
            try {
                message = JSON.parse(event.data);
            } catch (e) {
                return;
            }

            if (message && message.type === 'EpisodeTrackingUpdated') {
                updateEpisode(message.episodeID, message.tracking, message.infoUrl);
            }

            if (message && message.type === 'EpisodeSearchFinished') {
                onSearchFinished(message);
            }
        };

        socket.onclose = function () {
            setTimeout(connect, 3000);
        };

        socket.onerror = function () {
            socket.close();
        };
    }

    function addShowSearchPatternRow(pattern) {
        var container = document.getElementById('show-search-pattern-rows');
        if (!container) return;

        var row = document.createElement('div');
        row.className = 'search-pattern-row';

        var valueInput = document.createElement('input');
        valueInput.type = 'text';
        valueInput.className = 'search-pattern-value';
        valueInput.placeholder = '{alias} S{season:00}E{episode:00}...';
        valueInput.value = pattern || '';

        var removeBtn = document.createElement('button');
        removeBtn.type = 'button';
        removeBtn.className = 'btn-icon-sm btn-delete';
        removeBtn.title = 'Remove pattern';
        removeBtn.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-trash" viewBox="0 0 16 16"><path d="M5.5 5.5A.5.5 0 0 1 6 6v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5m2.5 0a.5.5 0 0 1 .5.5v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5m3 .5a.5.5 0 0 0-1 0v6a.5.5 0 0 0 1 0z"/><path d="M14.5 3a1 1 0 0 1-1 1H13v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V4h-.5a1 1 0 0 1-1-1V2a1 1 0 0 1 1-1H6a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1h3.5a1 1 0 0 1 1 1zM4.118 4 4 4.059V13a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1V4.059L11.882 4zM2.5 3h11V2h-11z"/></svg>';
        removeBtn.addEventListener('click', function () { row.remove(); });

        row.appendChild(valueInput);
        row.appendChild(removeBtn);
        container.appendChild(row);
    }

    function collectShowSearchPatterns() {
        var patterns = [];
        document.querySelectorAll('#show-search-pattern-rows .search-pattern-row').forEach(function (row) {
            var value = row.querySelector('.search-pattern-value').value.trim();
            if (value) patterns.push(value);
        });
        return patterns;
    }

    function saveShowSettings(form) {
        var showId = form.dataset.showId;
        if (!showId) return;

        var profileId = form.querySelector('[name=filter_profile_id]').value;
        var showTypeEl = form.querySelector('[name=show_type]');
        var showType = showTypeEl ? showTypeEl.value : 'standard';
        var payload = {
            filter_profile_id: profileId ? parseInt(profileId, 10) : null,
            show_type: showType,
            use_aliases: form.querySelector('[name=use_aliases]').checked,
            only_latin: form.querySelector('[name=only_latin]').checked,
            search_patterns: collectShowSearchPatterns()
        };

        fetch('/api/v1/database/show-settings/' + showId, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        })
            .then(function (resp) {
                return resp.json().then(function (body) { return { ok: resp.ok, body: body }; });
            })
            .then(function (result) {
                if (result.ok) {
                    displayToast((result.body && result.body.message) || 'Settings saved', 'success');
                    var container = form.closest('#modal-container');
                    if (container) container.remove();
                } else {
                    displayToast((result.body && result.body.message) || 'Failed to save settings', 'error');
                }
            })
            .catch(function () {
                displayToast('Failed to save settings', 'error');
            });
    }

    document.addEventListener('submit', function (event) {
        var form = event.target;
        if (form && form.id === 'edit-show-form') {
            event.preventDefault();
            saveShowSettings(form);
        }
    });

    function initSidebar() {
        var toggleBtn = document.getElementById('mobile-menu-toggle');
        var closeBtn = document.getElementById('sidebar-close-btn');
        var overlay = document.getElementById('mobile-overlay');
        var sidebar = document.getElementById('sidebar');

        function openSidebar() {
            document.body.classList.add('sidebar-open');
            if (sidebar) sidebar.classList.add('active');
            if (toggleBtn) toggleBtn.setAttribute('aria-expanded', 'true');
        }

        function closeSidebar() {
            document.body.classList.remove('sidebar-open');
            if (sidebar) sidebar.classList.remove('active');
            if (toggleBtn) toggleBtn.setAttribute('aria-expanded', 'false');
        }

        function toggleSidebar(e) {
            if (e) e.stopPropagation();
            if (document.body.classList.contains('sidebar-open')) {
                closeSidebar();
            } else {
                openSidebar();
            }
        }

        if (toggleBtn) {
            toggleBtn.addEventListener('click', toggleSidebar);
        }

        if (closeBtn) {
            closeBtn.addEventListener('click', function (e) {
                e.stopPropagation();
                closeSidebar();
            });
        }

        if (overlay) {
            overlay.addEventListener('click', function () {
                closeSidebar();
            });
        }

        // Close when clicking any menu link
        document.querySelectorAll('.sidebar-menu a').forEach(function (link) {
            link.addEventListener('click', function () {
                closeSidebar();
            });
        });

        // Close on Escape key
        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape' && document.body.classList.contains('sidebar-open')) {
                closeSidebar();
            }
        });

        // Close on resize if viewport expands beyond mobile breakpoint
        window.addEventListener('resize', function () {
            if (window.innerWidth > 768 && document.body.classList.contains('sidebar-open')) {
                closeSidebar();
            }
        });

        // Close on HTMX history/navigation
        document.body.addEventListener('htmx:historyRestore', closeSidebar);
        document.body.addEventListener('htmx:afterSwap', function (e) {
            if (e.detail && e.detail.target && e.detail.target.closest && e.detail.target.closest('.main-content')) {
                closeSidebar();
            }
            initTimezoneSelect();
        });
    }

    function initTimezoneSelect() {
        var wrapper = document.getElementById('tz-select-wrapper');
        if (!wrapper || wrapper.dataset.initialized === 'true') {
            return;
        }
        wrapper.dataset.initialized = 'true';

        var hiddenInput = document.getElementById('timezone');
        var trigger = document.getElementById('tz-select-trigger');
        var triggerLabel = document.getElementById('tz-trigger-label');
        var triggerBadge = document.getElementById('tz-trigger-badge');
        var dropdown = document.getElementById('tz-dropdown');
        var searchInput = document.getElementById('tz-search-input');
        var searchClear = document.getElementById('tz-search-clear');
        var optionsList = document.getElementById('tz-options-list');

        if (!hiddenInput || !trigger || !dropdown || !searchInput || !optionsList) {
            return;
        }

        var systemTz = wrapper.dataset.systemTz || 'Local';
        var currentTz = hiddenInput.value || '';

        function getTzOffset(tz) {
            if (!tz) return '';
            try {
                var now = new Date();
                var formatter = new Intl.DateTimeFormat('en-US', {
                    timeZone: tz,
                    timeZoneName: 'shortOffset'
                });
                var parts = formatter.formatToParts(now);
                var p = parts.find(function (part) { return part.type === 'timeZoneName'; });
                return p ? p.value.replace('GMT', 'UTC') : '';
            } catch (e) {
                return '';
            }
        }

        function getTzCity(tz) {
            if (!tz) return '';
            var slashIdx = tz.lastIndexOf('/');
            if (slashIdx !== -1) {
                return tz.substring(slashIdx + 1).replace(/_/g, ' ');
            }
            return tz;
        }

        var comprehensiveTzs = [
            // Americas
            'America/Adak', 'America/Anchorage', 'America/Anguilla', 'America/Antigua', 'America/Araguaina',
            'America/Argentina/Buenos_Aires', 'America/Argentina/Catamarca', 'America/Argentina/Cordoba', 'America/Argentina/Jujuy',
            'America/Argentina/La_Rioja', 'America/Argentina/Mendoza', 'America/Argentina/Rio_Gallegos', 'America/Argentina/Salta',
            'America/Argentina/San_Juan', 'America/Argentina/San_Luis', 'America/Argentina/Tucuman', 'America/Argentina/Ushuaia',
            'America/Aruba', 'America/Asuncion', 'America/Atikokan', 'America/Bahia', 'America/Bahia_Banderas', 'America/Barbados',
            'America/Belem', 'America/Belize', 'America/Blanc-Sablon', 'America/Boa_Vista', 'America/Bogota', 'America/Boise',
            'America/Buenos_Aires', 'America/Cambridge_Bay', 'America/Campo_Grande', 'America/Cancun', 'America/Caracas',
            'America/Cayenne', 'America/Cayman', 'America/Chicago', 'America/Chihuahua', 'America/Ciudad_Juarez', 'America/Costa_Rica',
            'America/Creston', 'America/Cuiaba', 'America/Curacao', 'America/Danmarkshavn', 'America/Dawson', 'America/Dawson_Creek',
            'America/Denver', 'America/Detroit', 'America/Dominica', 'America/Edmonton', 'America/Eirunepe', 'America/El_Salvador',
            'America/Fort_Nelson', 'America/Fortaleza', 'America/Glace_Bay', 'America/Goose_Bay', 'America/Grand_Turk',
            'America/Grenada', 'America/Guadeloupe', 'America/Guatemala', 'America/Guayaquil', 'America/Guyana', 'America/Halifax',
            'America/Havana', 'America/Hermosillo', 'America/Indiana/Indianapolis', 'America/Indiana/Knox', 'America/Indiana/Marengo',
            'America/Indiana/Petersburg', 'America/Indiana/Tell_City', 'America/Indiana/Vevay', 'America/Indiana/Vincennes', 'America/Indiana/Winamac',
            'America/Inuvik', 'America/Iqaluit', 'America/Jamaica', 'America/Jujuy', 'America/Juneau', 'America/Kentucky/Louisville',
            'America/Kentucky/Monticello', 'America/Kralendijk', 'America/La_Paz', 'America/Lima', 'America/Los_Angeles', 'America/Louisville',
            'America/Lower_Princes', 'America/Maceio', 'America/Managua', 'America/Manaus', 'America/Marigot', 'America/Martinique',
            'America/Matamoros', 'America/Mazatlan', 'America/Mendoza', 'America/Menominee', 'America/Merida', 'America/Metlakatla',
            'America/Mexico_City', 'America/Miquelon', 'America/Moncton', 'America/Monterrey', 'America/Montevideo', 'America/Montserrat',
            'America/Nassau', 'America/New_York', 'America/Nipigon', 'America/Nome', 'America/Noronha', 'America/North_Dakota/Beulah',
            'America/North_Dakota/Center', 'America/North_Dakota/New_Salem', 'America/Ojinaga', 'America/Panama', 'America/Pangnirtung',
            'America/Paramaribo', 'America/Phoenix', 'America/Port-au-Prince', 'America/Port_of_Spain', 'America/Porto_Velho',
            'America/Puerto_Rico', 'America/Punta_Arenas', 'America/Rainy_River', 'America/Rankin_Inlet', 'America/Recife',
            'America/Regina', 'America/Resolute', 'America/Rio_Branco', 'America/Santarem', 'America/Santiago', 'America/Santo_Domingo',
            'America/Sao_Paulo', 'America/Scoresbysund', 'America/Sitka', 'America/St_Barthelemy', 'America/St_Johns', 'America/St_Kitts',
            'America/St_Lucia', 'America/St_Thomas', 'America/St_Vincent', 'America/Swift_Current', 'America/Tegucigalpa', 'America/Thule',
            'America/Thunder_Bay', 'America/Tijuana', 'America/Toronto', 'America/Tortola', 'America/Vancouver', 'America/Whitehorse',
            'America/Winnipeg', 'America/Yakutat', 'America/Yellowknife',

            // Europe
            'Europe/Amsterdam', 'Europe/Andorra', 'Europe/Astrakhan', 'Europe/Athens', 'Europe/Belgrade', 'Europe/Berlin',
            'Europe/Bratislava', 'Europe/Brussels', 'Europe/Bucharest', 'Europe/Budapest', 'Europe/Busingen', 'Europe/Chisinau',
            'Europe/Copenhagen', 'Europe/Dublin', 'Europe/Gibraltar', 'Europe/Guernsey', 'Europe/Helsinki', 'Europe/Isle_of_Man',
            'Europe/Istanbul', 'Europe/Jersey', 'Europe/Kaliningrad', 'Europe/Kyiv', 'Europe/Kirov', 'Europe/Lisbon',
            'Europe/Ljubljana', 'Europe/London', 'Europe/Luxembourg', 'Europe/Madrid', 'Europe/Malta', 'Europe/Mariehamn',
            'Europe/Minsk', 'Europe/Monaco', 'Europe/Moscow', 'Europe/Oslo', 'Europe/Paris', 'Europe/Podgorica', 'Europe/Prague',
            'Europe/Riga', 'Europe/Rome', 'Europe/Samara', 'Europe/San_Marino', 'Europe/Sarajevo', 'Europe/Saratov', 'Europe/Simferopol',
            'Europe/Skopje', 'Europe/Sofia', 'Europe/Stockholm', 'Europe/Tallinn', 'Europe/Tirane', 'Europe/Ulyanovsk', 'Europe/Uzhgorod',
            'Europe/Vaduz', 'Europe/Vatican', 'Europe/Vienna', 'Europe/Vilnius', 'Europe/Volgograd', 'Europe/Warsaw', 'Europe/Zagreb',
            'Europe/Zaporozhye', 'Europe/Zurich',

            // Asia
            'Asia/Almaty', 'Asia/Amman', 'Asia/Anadyr', 'Asia/Aqtau', 'Asia/Aqtobe', 'Asia/Ashgabat', 'Asia/Atyrau', 'Asia/Baghdad',
            'Asia/Baku', 'Asia/Bangkok', 'Asia/Barnaul', 'Asia/Beirut', 'Asia/Bishkek', 'Asia/Brunei', 'Asia/Chita', 'Asia/Choibalsan',
            'Asia/Colombo', 'Asia/Damascus', 'Asia/Dhaka', 'Asia/Dili', 'Asia/Dubai', 'Asia/Dushanbe', 'Asia/Famagusta', 'Asia/Gaza',
            'Asia/Hebron', 'Asia/Ho_Chi_Minh', 'Asia/Hong_Kong', 'Asia/Hovd', 'Asia/Irkutsk', 'Asia/Jakarta', 'Asia/Jayapura',
            'Asia/Jerusalem', 'Asia/Kabul', 'Asia/Kamchatka', 'Asia/Karachi', 'Asia/Kathmandu', 'Asia/Khandyga', 'Asia/Kolkata',
            'Asia/Krasnoyarsk', 'Asia/Kuala_Lumpur', 'Asia/Kuching', 'Asia/Kuwait', 'Asia/Macau', 'Asia/Magadan', 'Asia/Makassar',
            'Asia/Manila', 'Asia/Muscat', 'Asia/Nicosia', 'Asia/Novokuznetsk', 'Asia/Novosibirsk', 'Asia/Omsk', 'Asia/Oral',
            'Asia/Phnom_Penh', 'Asia/Pontianak', 'Asia/Pyongyang', 'Asia/Qatar', 'Asia/Qostanay', 'Asia/Qyzylorda', 'Asia/Riyadh',
            'Asia/Sakhalin', 'Asia/Samarkand', 'Asia/Seoul', 'Asia/Shanghai', 'Asia/Singapore', 'Asia/Srednekolymsk', 'Asia/Taipei',
            'Asia/Tashkent', 'Asia/Tbilisi', 'Asia/Tehran', 'Asia/Thimphu', 'Asia/Tokyo', 'Asia/Tomsk', 'Asia/Ulaanbaatar',
            'Asia/Urumqi', 'Asia/Ust-Nera', 'Asia/Vientiane', 'Asia/Vladivostok', 'Asia/Yakutsk', 'Asia/Yangon', 'Asia/Yekaterinburg',
            'Asia/Yerevan',

            // Africa
            'Africa/Abidjan', 'Africa/Accra', 'Africa/Addis_Ababa', 'Africa/Algiers', 'Africa/Asmara', 'Africa/Bamako', 'Africa/Bangui',
            'Africa/Banjul', 'Africa/Bissau', 'Africa/Blantyre', 'Africa/Brazzaville', 'Africa/Bujumbura', 'Africa/Cairo', 'Africa/Casablanca',
            'Africa/Ceuta', 'Africa/Conakry', 'Africa/Dakar', 'Africa/Dar_es_Salaam', 'Africa/Djibouti', 'Africa/Douala', 'Africa/El_Aaiun',
            'Africa/Freetown', 'Africa/Gaborone', 'Africa/Harare', 'Africa/Johannesburg', 'Africa/Juba', 'Africa/Kampala', 'Africa/Khartoum',
            'Africa/Kigali', 'Africa/Kinshasa', 'Africa/Lagos', 'Africa/Libreville', 'Africa/Lome', 'Africa/Luanda', 'Africa/Lubumbashi',
            'Africa/Lusaka', 'Africa/Malabo', 'Africa/Maputo', 'Africa/Maseru', 'Africa/Mbabane', 'Africa/Mogadishu', 'Africa/Monrovia',
            'Africa/Nairobi', 'Africa/Ndjamena', 'Africa/Niamey', 'Africa/Nouakchott', 'Africa/Ouagadougou', 'Africa/Porto-Novo',
            'Africa/Sao_Tome', 'Africa/Tripoli', 'Africa/Tunis', 'Africa/Windhoek',

            // Pacific & Australia
            'Australia/Adelaide', 'Australia/Brisbane', 'Australia/Broken_Hill', 'Australia/Darwin', 'Australia/Eucla', 'Australia/Hobart',
            'Australia/Lindeman', 'Australia/Lord_Howe', 'Australia/Melbourne', 'Australia/Perth', 'Australia/Sydney',
            'Pacific/Apia', 'Pacific/Auckland', 'Pacific/Bougainville', 'Pacific/Chatham', 'Pacific/Chuuk', 'Pacific/Easter',
            'Pacific/Efate', 'Pacific/Fakaofo', 'Pacific/Fiji', 'Pacific/Funafuti', 'Pacific/Galapagos', 'Pacific/Gambier',
            'Pacific/Guadalcanal', 'Pacific/Guam', 'Pacific/Honolulu', 'Pacific/Kanton', 'Pacific/Kiritimati', 'Pacific/Kosrae',
            'Pacific/Kwajalein', 'Pacific/Majuro', 'Pacific/Marquesas', 'Pacific/Midway', 'Pacific/Nauru', 'Pacific/Niue',
            'Pacific/Norfolk', 'Pacific/Noumea', 'Pacific/Pago_Pago', 'Pacific/Palau', 'Pacific/Pitcairn', 'Pacific/Pohnpei',
            'Pacific/Port_Moresby', 'Pacific/Rarotonga', 'Pacific/Saipan', 'Pacific/Tahiti', 'Pacific/Tarawa', 'Pacific/Tongatapu',
            'Pacific/Wake', 'Pacific/Wallis',

            // Atlantic & Indian
            'Atlantic/Azores', 'Atlantic/Bermuda', 'Atlantic/Canary', 'Atlantic/Cape_Verde', 'Atlantic/Faroe', 'Atlantic/Madeira',
            'Atlantic/Reykjavik', 'Atlantic/South_Georgia', 'Atlantic/Stanley', 'Indian/Antananarivo', 'Indian/Chagos',
            'Indian/Christmas', 'Indian/Cocos', 'Indian/Comoro', 'Indian/Kerguelen', 'Indian/Mahe', 'Indian/Maldives',
            'Indian/Mauritius', 'Indian/Mayotte', 'Indian/Reunion',

            // UTC
            'UTC'
        ];

        var intlTzs = [];
        if (typeof Intl !== 'undefined' && Intl.supportedValuesOf) {
            try {
                intlTzs = Intl.supportedValuesOf('timeZone');
            } catch (e) {
                intlTzs = [];
            }
        }

        var allTzs = Array.from(new Set(comprehensiveTzs.concat(intlTzs))).sort();

        var popularTzs = [
            'America/Sao_Paulo', 'America/Fortaleza', 'America/Manaus',
            'America/New_York', 'America/Chicago', 'America/Los_Angeles',
            'UTC', 'Europe/London', 'Europe/Lisbon', 'Europe/Paris', 'Europe/Berlin',
            'Asia/Tokyo', 'Asia/Shanghai', 'Australia/Sydney'
        ];

        var groups = {
            'Popular': [],
            'Americas': [],
            'Europe': [],
            'Asia': [],
            'Pacific & Australia': [],
            'Africa': [],
            'Atlantic & Indian': [],
            'UTC & Other': []
        };

        popularTzs.forEach(function (tz) {
            if (allTzs.indexOf(tz) !== -1 || tz === 'UTC') {
                groups['Popular'].push(tz);
            }
        });

        allTzs.forEach(function (tz) {
            if (tz.startsWith('America/')) {
                groups['Americas'].push(tz);
            } else if (tz.startsWith('Europe/')) {
                groups['Europe'].push(tz);
            } else if (tz.startsWith('Asia/')) {
                groups['Asia'].push(tz);
            } else if (tz.startsWith('Australia/') || tz.startsWith('Pacific/')) {
                groups['Pacific & Australia'].push(tz);
            } else if (tz.startsWith('Africa/')) {
                groups['Africa'].push(tz);
            } else if (tz.startsWith('Atlantic/') || tz.startsWith('Indian/')) {
                groups['Atlantic & Indian'].push(tz);
            } else {
                groups['UTC & Other'].push(tz);
            }
        });

        var html = '';

        var isAutoSelected = !currentTz;
        html += '<div class="tz-group" data-group="auto">';
        html += '  <div class="tz-option' + (isAutoSelected ? ' selected' : '') + '" data-tz="" data-search="automatic system default auto ' + systemTz.toLowerCase() + '">';
        html += '    <div class="tz-option-main">';
        html += '      <span class="tz-option-name">Automatic (System Default)</span>';
        html += '      <span class="tz-option-sub">Uses host OS timezone (' + systemTz + ')</span>';
        html += '    </div>';
        html += '    <div class="tz-option-meta">';
        html += '      <span class="tz-badge badge-auto">Auto</span>';
        html += '      <svg class="tz-check-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7"></path></svg>';
        html += '    </div>';
        html += '  </div>';
        html += '</div>';

        function buildSearchKeywords(tz, city, offset, groupName) {
            var tokens = [
                tz.toLowerCase(),
                tz.replace(/_/g, ' ').toLowerCase(),
                tz.replace(/\//g, ' ').toLowerCase(),
                city.toLowerCase(),
                groupName.toLowerCase()
            ];

            if (offset) {
                var off = offset.toLowerCase();
                tokens.push(off);
                tokens.push(off.replace('utc', 'gmt'));
                tokens.push(off.replace('utc', ''));
                var numeric = off.replace('utc', '').replace(':00', '');
                tokens.push(numeric);
                if (numeric.startsWith('-0')) {
                    tokens.push('-' + numeric.substring(2));
                } else if (numeric.startsWith('+0')) {
                    tokens.push('+' + numeric.substring(2));
                }
            }

            // Country & regional aliases for quick search
            if (/Sao_Paulo|Fortaleza|Manaus|Recife|Bahia|Belem|Cuiaba|Campo_Grande|Porto_Velho|Boa_Vista|Rio_Branco|Maceio|Santarem|Noronha|Araguaina/.test(tz)) {
                tokens.push('brazil brasil brt');
            } else if (/Buenos_Aires|Cordoba|Mendoza|Catamarca|Jujuy|Salta|San_Juan|Tucuman|Ushuaia/.test(tz)) {
                tokens.push('argentina art');
            } else if (/Santiago|Punta_Arenas/.test(tz)) {
                tokens.push('chile clt');
            } else if (/Bogota/.test(tz)) {
                tokens.push('colombia');
            } else if (/Lima/.test(tz)) {
                tokens.push('peru');
            } else if (/Montevideo/.test(tz)) {
                tokens.push('uruguay');
            } else if (/Mexico_City|Cancun|Monterrey|Tijuana|Ciudad_Juarez|Chihuahua|Hermosillo|Mazatlan/.test(tz)) {
                tokens.push('mexico');
            } else if (/New_York|Chicago|Los_Angeles|Denver|Phoenix|Detroit|Indianapolis|Anchorage|Honolulu|Boise/.test(tz)) {
                tokens.push('usa united states america');
            } else if (/Toronto|Vancouver|Montreal|Edmonton|Winnipeg|Halifax|St_Johns|Calgary/.test(tz)) {
                tokens.push('canada');
            } else if (/London/.test(tz)) {
                tokens.push('uk united kingdom britain england gmt bst');
            } else if (/Lisbon|Madeira|Azores/.test(tz)) {
                tokens.push('portugal');
            } else if (/Madrid|Ceuta|Canary/.test(tz)) {
                tokens.push('spain espana');
            } else if (/Paris/.test(tz)) {
                tokens.push('france');
            } else if (/Berlin|Busingen/.test(tz)) {
                tokens.push('germany deutschland');
            } else if (/Rome/.test(tz)) {
                tokens.push('italy italia');
            } else if (/Tokyo/.test(tz)) {
                tokens.push('japan jst');
            } else if (/Seoul/.test(tz)) {
                tokens.push('korea');
            } else if (/Shanghai|Beijing|Hong_Kong|Macau|Urumqi/.test(tz)) {
                tokens.push('china');
            } else if (/Sydney|Melbourne|Brisbane|Perth|Adelaide|Darwin|Hobart/.test(tz)) {
                tokens.push('australia');
            } else if (/Auckland|Chatham/.test(tz)) {
                tokens.push('new zealand');
            }

            return tokens.join(' ');
        }

        Object.keys(groups).forEach(function (groupName) {
            var list = groups[groupName];
            if (!list || list.length === 0) return;

            html += '<div class="tz-group" data-group="' + groupName.toLowerCase() + '">';
            html += '  <div class="tz-group-title">' + groupName + ' (' + list.length + ')</div>';

            list.forEach(function (tz) {
                var offset = getTzOffset(tz);
                var city = getTzCity(tz);
                var isSelected = currentTz === tz;
                var searchStr = buildSearchKeywords(tz, city, offset, groupName);

                html += '  <div class="tz-option' + (isSelected ? ' selected' : '') + '" data-tz="' + tz + '" data-offset="' + offset + '" data-search="' + searchStr + '">';
                html += '    <div class="tz-option-main">';
                html += '      <span class="tz-option-name">' + tz + '</span>';
                html += '      <span class="tz-option-sub">' + city + '</span>';
                html += '    </div>';
                html += '    <div class="tz-option-meta">';
                if (offset) {
                    html += '      <span class="tz-badge badge-offset">' + offset + '</span>';
                }
                html += '      <svg class="tz-check-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7"></path></svg>';
                html += '    </div>';
                html += '  </div>';
            });

            html += '</div>';
        });

        html += '<div class="tz-empty" id="tz-empty" style="display: none;">No timezones found matching your search.</div>';
        optionsList.innerHTML = html;

        function selectTimezone(tz, offset) {
            hiddenInput.value = tz;
            if (!tz) {
                triggerLabel.textContent = 'Automatic (System: ' + systemTz + ')';
                triggerBadge.textContent = 'Auto';
                triggerBadge.className = 'tz-badge badge-auto';
            } else {
                var displayOffset = offset || getTzOffset(tz);
                triggerLabel.textContent = tz + (displayOffset ? ' (' + displayOffset + ')' : '');
                triggerBadge.textContent = displayOffset || 'Custom';
                triggerBadge.className = 'tz-badge badge-offset';
            }

            optionsList.querySelectorAll('.tz-option').forEach(function (opt) {
                opt.classList.toggle('selected', opt.dataset.tz === tz);
                opt.classList.remove('highlighted');
            });

            closeDropdown();
        }

        function updatePlacement() {
            var rect = wrapper.getBoundingClientRect();
            var spaceBelow = window.innerHeight - rect.bottom;
            var spaceAbove = rect.top;
            var dropdownHeight = 390;

            if (spaceBelow < dropdownHeight && spaceAbove > 260) {
                wrapper.classList.add('dropup');
            } else {
                wrapper.classList.remove('dropup');
            }
        }

        function openDropdown() {
            wrapper.classList.add('open');
            trigger.setAttribute('aria-expanded', 'true');
            var section = wrapper.closest('.settings-section');
            if (section) {
                section.style.zIndex = '100';
            }
            updatePlacement();
            searchInput.value = '';
            searchClear.style.display = 'none';
            filterOptions('');
            setTimeout(function () {
                searchInput.focus();
                try {
                    dropdown.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                } catch (e) {}
            }, 60);
        }

        function closeDropdown() {
            wrapper.classList.remove('open');
            wrapper.classList.remove('dropup');
            trigger.setAttribute('aria-expanded', 'false');
            var section = wrapper.closest('.settings-section');
            if (section) {
                section.style.zIndex = '';
            }
        }

        window.addEventListener('resize', function () {
            if (wrapper.classList.contains('open')) {
                updatePlacement();
            }
        });

        trigger.addEventListener('click', function (e) {
            e.preventDefault();
            e.stopPropagation();
            if (wrapper.classList.contains('open')) {
                closeDropdown();
            } else {
                openDropdown();
            }
        });

        function getVisibleOptions() {
            return Array.from(optionsList.querySelectorAll('.tz-option')).filter(function (opt) {
                return opt.style.display !== 'none' && opt.closest('.tz-group').style.display !== 'none';
            });
        }

        function filterOptions(query) {
            var clean = (query || '').trim().toLowerCase();
            var visibleCount = 0;
            var groupsEl = optionsList.querySelectorAll('.tz-group');

            groupsEl.forEach(function (groupEl) {
                var options = groupEl.querySelectorAll('.tz-option');
                var groupVisible = 0;

                options.forEach(function (opt) {
                    opt.classList.remove('highlighted');
                    var searchData = opt.dataset.search || '';
                    var matches = !clean || searchData.indexOf(clean) !== -1;
                    opt.style.display = matches ? 'flex' : 'none';
                    if (matches) {
                        groupVisible++;
                        visibleCount++;
                    }
                });

                groupEl.style.display = groupVisible > 0 ? 'block' : 'none';
            });

            var emptyEl = document.getElementById('tz-empty');
            if (emptyEl) {
                emptyEl.style.display = visibleCount === 0 ? 'block' : 'none';
            }
        }

        searchInput.addEventListener('input', function () {
            var q = searchInput.value;
            searchClear.style.display = q ? 'block' : 'none';
            filterOptions(q);
        });

        searchClear.addEventListener('click', function (e) {
            e.stopPropagation();
            searchInput.value = '';
            searchClear.style.display = 'none';
            filterOptions('');
            searchInput.focus();
        });

        optionsList.addEventListener('click', function (e) {
            var opt = e.target.closest('.tz-option');
            if (!opt) return;
            var tz = opt.dataset.tz || '';
            var offset = opt.dataset.offset || '';
            selectTimezone(tz, offset);
        });

        searchInput.addEventListener('keydown', function (e) {
            if (e.key === 'Escape') {
                closeDropdown();
                trigger.focus();
            } else if (e.key === 'ArrowDown') {
                e.preventDefault();
                var visible = getVisibleOptions();
                if (visible.length === 0) return;
                var currentIdx = visible.findIndex(function (el) { return el.classList.contains('highlighted'); });
                var nextIdx = currentIdx < visible.length - 1 ? currentIdx + 1 : 0;
                visible.forEach(function (el) { el.classList.remove('highlighted'); });
                visible[nextIdx].classList.add('highlighted');
                visible[nextIdx].scrollIntoView({ block: 'nearest' });
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                var visible = getVisibleOptions();
                if (visible.length === 0) return;
                var currentIdx = visible.findIndex(function (el) { return el.classList.contains('highlighted'); });
                var prevIdx = currentIdx > 0 ? currentIdx - 1 : visible.length - 1;
                visible.forEach(function (el) { el.classList.remove('highlighted'); });
                visible[prevIdx].classList.add('highlighted');
                visible[prevIdx].scrollIntoView({ block: 'nearest' });
            } else if (e.key === 'Enter') {
                e.preventDefault();
                var highlighted = optionsList.querySelector('.tz-option.highlighted');
                if (!highlighted) {
                    var visible = getVisibleOptions();
                    if (visible.length > 0) highlighted = visible[0];
                }
                if (highlighted) {
                    selectTimezone(highlighted.dataset.tz || '', highlighted.dataset.offset || '');
                }
            }
        });

        document.addEventListener('click', function (e) {
            if (!wrapper.contains(e.target)) {
                closeDropdown();
            }
        });

        if (currentTz) {
            var curOffset = getTzOffset(currentTz);
            if (curOffset) {
                triggerLabel.textContent = currentTz + ' (' + curOffset + ')';
                triggerBadge.textContent = curOffset;
            }
        }
    }

    function init() {
        connect();
        initSidebar();
        initTimezoneSelect();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    window.showToast = showToast;
    window.toast = displayToast;
    window.handleAliasAdded = handleAliasAdded;
    window.addShowSearchPatternRow = addShowSearchPatternRow;
})();
