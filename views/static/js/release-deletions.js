(function (root, factory) {
    var api = factory();
    if (typeof module === 'object' && module.exports) module.exports = api;
    else root.ReleaseDeletions = api;
})(typeof window !== 'undefined' ? window : this, function () {
    'use strict';
    function createState() { return { operations: {}, followed: {}, latest: {} }; }
    function follow(state, id) { state.followed[id] = true; }
    function description(op) {
        if (op.status === 'completed') return { type: 'success', message: 'Release deleted' };
        if (op.status === 'failed' && op.next_attempt_at > 0 && op.retryable) return { type: 'info', message: 'Release deletion is incomplete; an automatic retry is scheduled' };
        if (op.status === 'failed') return { type: 'error', message: 'Release deletion is incomplete. Check its status and retry.' };
        return { type: 'info', message: 'Release deletion accepted; cleanup is pending' };
    }
    function apply(state, op) {
        if (!op || !op.id) return null;
        var previous = state.operations[op.id];
        if (previous && Number(previous.version) >= Number(op.version)) return null;
        state.operations[op.id] = op;
        var target = op.episode_id ? 'episode:' + op.episode_id : op.target || (op.hash ? 'hash:' + op.hash : op.id);
        var latest = state.latest[target];
        var current = !latest || latest.id === op.id || Number(op.created_at || 0) > Number(latest.created_at || 0);
        if (current) state.latest[target] = op;
        var changed = !previous || description(previous).message !== description(op).message;
        return { operation: op, current: current, notification: current && state.followed[op.id] && changed ? description(op) : null };
    }
    function attach(notify, updateEpisode) {
        var state = createState();
        var targets = {};
        var notifiedBatches = {};
        try { targets = JSON.parse(sessionStorage.getItem('release-deletion-targets') || '{}'); } catch (e) {}
        function remember(key) { if (!key) return; targets[key] = true; try { sessionStorage.setItem('release-deletion-targets', JSON.stringify(targets)); } catch (e) {} }
        function key(op) { return op.episode_id ? 'episode:' + op.episode_id : 'hash:' + op.hash; }
        function cards(op) {
            var selector = op.episode_id ? '[data-episode-id="' + Number(op.episode_id) + '"]' : '';
            if (op.hash && /^[a-zA-Z0-9]+$/.test(op.hash)) selector += (selector ? ',' : '') + '[data-download-hash="' + op.hash + '"]';
            return selector ? document.querySelectorAll(selector) : [];
        }
        function retryOperation(op, button) {
            button.disabled = true; follow(state, op.id); remember(key(op));
            return fetch('/api/v1/release-deletions/' + encodeURIComponent(op.id) + '/retry', { method: 'POST' }).then(function (response) {
                if (!response.ok) throw new Error('retry rejected'); return response.json();
            }).then(function (response) {
                if (!consumeResponse(response) || !response.operation || response.operation.status === 'failed') throw new Error('retry not accepted');
            }).catch(function () { button.disabled = false; notify('Unable to retry deletion; please try again', 'error'); });
        }
        function render(op) {
            var panel = document.getElementById('release-deletion-operations');
            var row = document.getElementById('release-deletion-' + op.id);
            if (op.status !== 'completed') {
                if (!panel) { panel = document.createElement('aside'); panel.id = 'release-deletion-operations'; panel.setAttribute('aria-label', 'Release deletions'); document.body.appendChild(panel); }
                if (!row) { row = document.createElement('div'); row.id = 'release-deletion-' + op.id; row.className = 'deletion-state'; panel.appendChild(row); }
                row.textContent = (op.status === 'failed' ? 'Deletion incomplete' : 'Deletion pending') + ' · ' + (op.episode_id ? 'Episode ' + op.episode_id : op.hash) + ' · ' + op.stage;
                if (op.status === 'failed') { var panelRetry = document.createElement('button'); panelRetry.type = 'button'; panelRetry.textContent = 'Retry deletion'; panelRetry.addEventListener('click', function () { retryOperation(op, panelRetry); }); row.appendChild(panelRetry); }
            } else if (row) row.remove();
            cards(op).forEach(function (card) {
                var existing = card.querySelector('.deletion-state');
                var active = op.status !== 'completed';
                card.querySelectorAll('button,select,input,[hx-delete],[hx-post]').forEach(function (control) {
                    if (control.classList.contains('deletion-retry')) return;
                    if (active && !control.disabled) { control.dataset.deletionDisabled = 'true'; control.disabled = true; }
                    if (!active && control.dataset.deletionDisabled) { control.disabled = false; delete control.dataset.deletionDisabled; }
                });
                card.dataset.deletionPending = active ? 'true' : 'false';
                if (!active) { if (existing) existing.remove(); return; }
                if (!existing) { existing = document.createElement('div'); existing.className = 'deletion-state'; existing.setAttribute('role', 'status'); card.appendChild(existing); }
                existing.textContent = (op.status === 'failed' ? 'Deletion incomplete' : 'Deletion pending') + ' · ' + op.stage;
                if (op.status === 'failed') {
                    var retry = document.createElement('button'); retry.type = 'button'; retry.className = 'deletion-retry'; retry.textContent = 'Retry deletion';
                    retry.addEventListener('click', function (event) {
                        event.stopPropagation(); retryOperation(op, retry);
                    }); existing.appendChild(retry);
                }
            });
        }
        function consume(op, followed, live) {
            if (!op || !op.id) return;
            if (followed || targets[key(op)] || targets['hash:' + op.hash]) follow(state, op.id);
            var result = apply(state, op); if (!result) return;
            if (result.current) render(op);
            // Persisted deletion history is not authoritative episode tracking.
            if (live !== false && result.current && op.status === 'completed' && op.episode_id) updateEpisode(op.episode_id, op.intention === 'blacklist' ? 'wanted' : op.intention, '');
            if (result.notification) notify(result.notification.message, result.notification.type);
            if (op.status === 'completed') { delete targets[key(op)]; delete targets['hash:' + op.hash]; try { sessionStorage.setItem('release-deletion-targets', JSON.stringify(targets)); } catch (e) {} }
        }
        function consumeResponse(response) {
            var data = response.data; if (Array.isArray(data)) data = data[0];
            if ((response.operation && response.operation.id) || (data && data.operation && data.operation.id)) { consume(response.operation || data.operation, true); return true; }
            if (response.results) {
                var found = false;
                response.results.forEach(function (result) { if (result.operation && result.operation.id) { consume(result.operation, true); found = true; } });
                if (found) {
                    var batchKey = JSON.stringify(response.results);
                    if (!notifiedBatches[batchKey]) response.results.forEach(function (result) {
                        if (result.error) notify('Episode ' + result.episode_id + ': ' + result.error, 'error');
                    });
                    notifiedBatches[batchKey] = true;
                }
                return found;
            }
            return false;
        }
        function refresh() {
            return fetch('/api/v1/release-deletions').then(function (response) { if (!response.ok) throw new Error('query failed'); return response.json(); }).then(function (data) {
                // Render older operations first so the newest request for a target wins.
                (data.operations || []).slice().reverse().forEach(function (op) { consume(op, false, false); });
                (data.operations || []).forEach(function (op) { var latest = state.latest[op.episode_id ? key(op) : op.target || key(op)]; if (latest && latest.id === op.id && op.episode_id && op.current_tracking) updateEpisode(op.episode_id, op.current_tracking, ''); });
            }).catch(function () {});
        }
        function reapply() { Object.values(state.latest).forEach(render); }
        document.body.addEventListener('htmx:beforeRequest', function (event) {
            var config = event.detail.requestConfig || {};
            var path = config.path || ''; var verb = String(config.verb || '').toLowerCase();
            if (!(/\/front\/downloads\/(remove|blacklist)$/.test(path) || (verb === 'delete' && /\/database\/show\/episode\/\d+$/.test(path)) || /\/database\/show\/episode\/status$/.test(path))) return;
            var elt = event.detail.elt || event.target;
            var card = elt.closest('[data-episode-id],[data-download-hash]');
            if (card) remember(card.dataset.episodeId ? 'episode:' + card.dataset.episodeId : 'hash:' + card.dataset.downloadHash);
        });
        document.body.addEventListener('htmx:afterRequest', function (event) { try { consumeResponse(JSON.parse(event.detail.xhr.response)); } catch (e) {} });
        document.body.addEventListener('htmx:afterSwap', function () { reapply(); refresh(); });
        refresh();
        return { consume: consume, consumeResponse: consumeResponse, refresh: refresh, reapply: reapply };
    }
    return { createState: createState, follow: follow, apply: apply, description: description, attach: attach };
});
