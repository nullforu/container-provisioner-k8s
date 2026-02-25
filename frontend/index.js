const defaultPodSpec = `apiVersion: v1
kind: Pod
metadata:
  name: challenge
spec:
  containers:
    - name: app
      image: nginx:stable
      ports:
        - containerPort: 80
          protocol: TCP
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
        limits:
          cpu: "100m"
          memory: "128Mi"`

function byId(id) {
    return document.getElementById(id)
}

function apiBase() {
    const el = byId('apiBase')
    const raw = el ? el.value.trim() : ''
    if (raw.length > 0) return raw.replace(/\/+$/, '')
    return window.location.origin
}

function apiKeyValue() {
    const el = byId('apiKeyValue')
    return el ? el.value.trim() : ''
}

function readInt(id) {
    const el = byId(id)
    return Number.parseInt(el ? el.value : '0', 10)
}

function readTargetPort(id) {
    const el = byId(id)
    const raw = el ? el.value.trim() : ''
    if (!raw) return null
    if (!raw.startsWith('[')) {
        throw new Error('target_port must be a JSON array')
    }
    try {
        return JSON.parse(raw)
    } catch (err) {
        throw new Error(`invalid target_port json: ${err.message}`)
    }
}

function syncStackID(stackID) {
    if (!stackID) return
    const last = byId('lastStackId')
    const input = byId('stackIdInput')
    if (last) last.value = stackID
    if (input) input.value = stackID
}

function showResponse(title, payload, state = 'ok') {
    const el = byId('response')
    if (!el) return
    const ts = new Date().toISOString()
    const body = typeof payload === 'string' ? payload : JSON.stringify(payload, null, 2)
    el.textContent = `[${ts}] ${title}\n\n${body}`
    el.dataset.state = state
}

function formatISO(value) {
    if (!value) return '-'
    const d = new Date(value)
    if (Number.isNaN(d.getTime())) return value
    return d.toISOString()
}

function formatBytes(bytes) {
    const n = Number(bytes)
    if (!Number.isFinite(n)) return '-'
    if (n < 1024) return `${n} B`
    const kb = n / 1024
    if (kb < 1024) return `${kb.toFixed(1)} KB`
    const mb = kb / 1024
    if (mb < 1024) return `${mb.toFixed(1)} MB`
    const gb = mb / 1024
    return `${gb.toFixed(2)} GB`
}

function formatMilliCPU(milli) {
    const n = Number(milli)
    if (!Number.isFinite(n)) return '-'
    return `${n}m`
}

function renderStacks(stacks) {
    const list = byId('stackList')
    if (!list) return
    list.innerHTML = ''

    if (!Array.isArray(stacks) || stacks.length === 0) {
        const empty = document.createElement('div')
        empty.className = 'stackEmpty'
        empty.textContent = 'No stacks found.'
        list.appendChild(empty)
        return
    }

    stacks.forEach((st) => {
        const card = document.createElement('article')
        card.className = 'stackCard'
        card.dataset.stackId = st.stack_id || ''

        const header = document.createElement('div')
        header.className = 'stackCardHead'

        const title = document.createElement('div')
        title.className = 'stackTitle'
        title.textContent = st.stack_id || 'stack'

        const status = document.createElement('div')
        status.className = `stackStatus status-${(st.status || '').toLowerCase()}`
        status.textContent = st.status || 'unknown'

        const actions = document.createElement('div')
        actions.className = 'stackCardActions'
        const del = document.createElement('button')
        del.className = 'action danger'
        del.type = 'button'
        del.setAttribute('data-action', 'deleteStackItem')
        del.setAttribute('data-stack-id', st.stack_id || '')
        del.textContent = 'Delete'
        actions.appendChild(del)

        header.appendChild(title)
        header.appendChild(status)
        header.appendChild(actions)

        const grid = document.createElement('div')
        grid.className = 'stackMeta'

        const entries = [
            ['namespace', st.namespace],
            ['pod_id', st.pod_id],
            ['service_name', st.service_name],
            ['node_id', st.node_id],
            ['node_public_ip', st.node_public_ip || '-'],
            ['ports', JSON.stringify(st.ports || null)],
            ['ttl_expires_at', formatISO(st.ttl_expires_at)],
            ['created_at', formatISO(st.created_at)],
            ['updated_at', formatISO(st.updated_at)],
            ['requested_cpu', formatMilliCPU(st.requested_cpu_milli)],
            ['requested_memory', formatBytes(st.requested_memory_bytes)],
        ]

        entries.forEach(([key, value]) => {
            const row = document.createElement('div')
            row.className = 'stackMetaRow'
            const k = document.createElement('div')
            k.className = 'stackMetaKey'
            k.textContent = key
            const v = document.createElement('div')
            v.className = 'stackMetaValue'
            v.textContent = value === undefined || value === null || value === '' ? '-' : String(value)
            row.appendChild(k)
            row.appendChild(v)
            grid.appendChild(row)
        })

        card.appendChild(header)
        card.appendChild(grid)
        list.appendChild(card)
    })
}

async function request(method, path, body) {
    const url = `${apiBase()}${path}`
    const options = { method, headers: {} }

    const key = apiKeyValue()
    if (key) {
        options.headers['X-API-KEY'] = key
    }

    if (body !== undefined) {
        options.headers['Content-Type'] = 'application/json'
        options.body = JSON.stringify(body)
    }

    const res = await fetch(url, options)
    const raw = await res.text()

    let parsed = raw
    try {
        parsed = raw.length ? JSON.parse(raw) : {}
    } catch (_) {}

    const responsePayload = {
        method,
        url,
        status: res.status,
        ok: res.ok,
        body: parsed,
    }

    if (!res.ok) throw responsePayload
    return responsePayload
}

function setBusy(on) {
    document.body.dataset.busy = on ? 'true' : 'false'
    document.querySelectorAll('button[data-action]').forEach((b) => {
        b.disabled = on
    })
}

async function execute(title, fn) {
    try {
        setBusy(true)
        showResponse(title, { status: 'running' }, 'idle')
        const result = await fn()
        showResponse(title, result, 'ok')
    } catch (err) {
        if (err instanceof Error) {
            showResponse(`${title} (ERROR)`, { error: err.message }, 'error')
            return
        }
        showResponse(`${title} (ERROR)`, err, 'error')
    } finally {
        setBusy(false)
    }
}

function getStackID() {
    const el = byId('stackIdInput')
    const stackID = el ? el.value.trim() : ''
    if (!stackID) throw new Error('stack_id is required')
    return stackID
}

function setActiveTab(tab) {
    document.querySelectorAll('[data-tab]').forEach((t) => {
        const active = t.getAttribute('data-tab') === tab
        t.classList.toggle('active', active)
        t.setAttribute('aria-selected', active ? 'true' : 'false')
    })

    document.querySelectorAll('[data-panel]').forEach((p) => {
        const active = p.getAttribute('data-panel') === tab
        p.classList.toggle('active', active)
    })

    try {
        localStorage.setItem('cpdash.activeTab', tab)
    } catch (_) {}
}

function wireTabs() {
    const tabBar = byId('tabBar')
    if (!tabBar) return

    tabBar.addEventListener('click', (e) => {
        const btn = e.target.closest('[data-tab]')
        if (!btn) return
        setActiveTab(btn.getAttribute('data-tab'))
    })

    tabBar.addEventListener('keydown', (e) => {
        if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
        const tabs = Array.from(tabBar.querySelectorAll('[data-tab]'))
        const current = document.activeElement.closest('[data-tab]')
        const idx = Math.max(0, tabs.indexOf(current))
        const next = e.key === 'ArrowRight' ? idx + 1 : idx - 1
        const target = tabs[(next + tabs.length) % tabs.length]
        target.focus()
        setActiveTab(target.getAttribute('data-tab'))
        e.preventDefault()
    })

    let initial = 'settings'
    try {
        initial = localStorage.getItem('cpdash.activeTab') || initial
    } catch (_) {}
    setActiveTab(initial)
}

function wireActions() {
    document.addEventListener('click', (e) => {
        const btn = e.target.closest('button[data-action]')
        if (!btn) return

        const action = btn.getAttribute('data-action')

        if (action === 'health') execute('GET /healthz', () => request('GET', '/healthz'))
        else if (action === 'listStacks') execute('GET /stacks', () => request('GET', '/stacks'))
        else if (action === 'stats') execute('GET /stats', () => request('GET', '/stats'))
        else if (action === 'create') {
            execute('POST /stacks', async () => {
                const payload = {
                    target_port: readTargetPort('createTargetPort'),
                    pod_spec: byId('createPodSpec')?.value ?? '',
                }
                const result = await request('POST', '/stacks', payload)
                const stackID = result?.body?.stack_id ?? ''
                syncStackID(stackID)
                return result
            })
        } else if (action === 'getStack')
            execute('GET /stacks/{stack_id}', () => request('GET', `/stacks/${getStackID()}`))
        else if (action === 'getStatus')
            execute('GET /stacks/{stack_id}/status', () => request('GET', `/stacks/${getStackID()}/status`))
        else if (action === 'deleteStack')
            execute('DELETE /stacks/{stack_id}', () => request('DELETE', `/stacks/${getStackID()}`))
        else if (action === 'refreshStacks') {
            execute('GET /stacks (refresh)', async () => {
                const result = await request('GET', '/stacks')
                renderStacks(result?.body?.stacks || [])
                return result
            })
        } else if (action === 'deleteStackItem') {
            const stackID = btn.getAttribute('data-stack-id') || ''
            if (!stackID) return
            execute(`DELETE /stacks/${stackID}`, async () => {
                const result = await request('DELETE', `/stacks/${stackID}`)
                const refreshed = await request('GET', '/stacks')
                renderStacks(refreshed?.body?.stacks || [])
                return result
            })
        } else if (action === 'clearResponse') {
            const r = byId('response')
            if (r) {
                r.textContent = 'ready'
                r.dataset.state = 'idle'
            }
        }
    })
}

function boot() {
    const api = byId('apiBase')
    const spec = byId('createPodSpec')
    const resp = byId('response')
    const apiKeyValueEl = byId('apiKeyValue')
    const apiKeyStatusEl = byId('apiKeyStatus')

    if (api) api.value = window.location.origin
    if (spec) spec.value = defaultPodSpec
    if (resp) resp.dataset.state = 'idle'

    const updateAPIKeyStatus = () => {
        if (!apiKeyStatusEl) return
        apiKeyStatusEl.value = apiKeyValue() ? 'enabled' : 'disabled'
    }

    try {
        if (apiKeyValueEl) {
            apiKeyValueEl.value = localStorage.getItem('cpdash.apiKeyValue') || ''
            apiKeyValueEl.addEventListener('input', () => {
                localStorage.setItem('cpdash.apiKeyValue', apiKeyValueEl.value)
                updateAPIKeyStatus()
            })
        }
    } catch (_) {}
    updateAPIKeyStatus()

    wireTabs()
    wireActions()
}

window.addEventListener('DOMContentLoaded', boot)
