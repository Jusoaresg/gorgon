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
        });
    }

    function init() {
        connect();
        initSidebar();
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
