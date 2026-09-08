import { useCallback, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { uploadExport } from '../api/client'
import { useImports, usePeople, usePeopleMutations, useUploadStatus } from '../api/hooks'
import type { Person } from '../api/types'
import { Dialog } from '../components/Dialog'
import { Avatar } from '../components/PersonSwitcher'
import { ErrorNote, Loading, Section } from '../components/Section'
import { fmtBytes, fmtDate, fmtDateTime, fmtInt } from '../format'

const EMOJIS = ['🙂', '😀', '😎', '🧑', '👩', '👨', '👧', '👦', '👶', '🧓', '👵', '👴', '🏃', '🚴', '🧗', '🏊', '🧘', '💪', '🐱', '🐶', '🦊', '🐼', '🦄', '🌻', '⭐', '🔥', '🌊', '🍀']
const COLORS = ['#e4572e', '#17bebb', '#ffc914', '#76b041', '#8a4fff', '#ff7f51', '#3c91e6', '#f06292', '#26a69a', '#8d6e63']

export function PeoplePage() {
  const people = usePeople()
  const { create, update, remove } = usePeopleMutations()
  const [editing, setEditing] = useState<Partial<Person> | null>(null)
  const [deleting, setDeleting] = useState<Person | null>(null)
  const [confirmName, setConfirmName] = useState('')

  const empty = people.data && people.data.length === 0

  return (
    <>
      <div className="page-head">
        <div>
          <h1>People & imports</h1>
          <div className="sub">Each person has their own database. Upload a full Apple Health export (export.zip) to import or refresh their data.</div>
        </div>
        <button className="btn primary" onClick={() => setEditing({})}>+ Add person</button>
      </div>

      {people.isLoading && <Loading />}
      {people.error && <ErrorNote error={people.error} />}
      {empty && (
        <div className="card empty">
          <div className="big">👋</div>
          <div style={{ fontWeight: 600, color: 'var(--text)' }}>Welcome to healthsync</div>
          <div style={{ marginTop: 6 }}>Add the first family member, then upload their Apple Health export. On the iPhone: Health app → profile picture → Export All Health Data.</div>
          <button className="btn primary" style={{ marginTop: 16 }} onClick={() => setEditing({})}>Add the first person</button>
        </div>
      )}

      <div className="stack">
        {people.data?.map((p) => (
          <PersonCard key={p.id} p={p} onEdit={() => setEditing(p)} onDelete={() => { setDeleting(p); setConfirmName('') }} />
        ))}
      </div>

      {editing && (
        <PersonDialog
          initial={editing}
          busy={create.isPending || update.isPending}
          error={create.error ?? update.error}
          onClose={() => setEditing(null)}
          onSave={async (p) => {
            if (editing.id) await update.mutateAsync({ ...p, id: editing.id })
            else await create.mutateAsync(p)
            setEditing(null)
          }}
        />
      )}
      {deleting && (
        <Dialog title={`Delete ${deleting.name}?`} onClose={() => setDeleting(null)}>
          <p className="muted">This permanently deletes their database ({fmtInt(deleting.total_rows)} rows). Type the name to confirm.</p>
          <div className="field" style={{ marginTop: 12 }}>
            <input type="text" value={confirmName} onChange={(e) => setConfirmName(e.target.value)} placeholder={deleting.name} autoFocus />
          </div>
          {remove.error && <ErrorNote error={remove.error} />}
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            <button className="btn" onClick={() => setDeleting(null)}>Cancel</button>
            <button className="btn danger" disabled={confirmName.trim().toLowerCase() !== deleting.name.toLowerCase() || remove.isPending} onClick={async () => { await remove.mutateAsync(deleting.id); setDeleting(null) }}>
              Delete
            </button>
          </div>
        </Dialog>
      )}
    </>
  )
}

function PersonCard({ p, onEdit, onDelete }: { p: Person; onEdit: () => void; onDelete: () => void }) {
  const [uploading, setUploading] = useState<number | null>(null)
  const [uploadErr, setUploadErr] = useState<string | null>(null)
  const status = useUploadStatus(p.id, uploading !== null)
  const imports = useImports(p.id)
  const inputRef = useRef<HTMLInputElement>(null)
  const [over, setOver] = useState(false)

  const send = useCallback(
    async (file: File) => {
      setUploadErr(null)
      setUploading(0)
      try {
        await uploadExport(p.id, file, setUploading)
        setUploading(null)
        status.refetch()
      } catch (e) {
        setUploading(null)
        setUploadErr((e as Error).message)
      }
    },
    [p.id, status],
  )

  const st = status.data
  const running = st?.status === 'running' || uploading !== null

  return (
    <Section
      title={
        <span className="row">
          <Avatar p={p} size="lg" />
          <span>
            <div style={{ fontSize: 18 }}>{p.name}</div>
            <div className="muted small" style={{ fontWeight: 400 }}>
              {p.dob && `born ${fmtDate(p.dob)} · `}
              {p.sex && `${p.sex} · `}
              {p.has_data ? <>data {fmtDate(p.first_date)} – {fmtDate(p.last_date)} · {fmtInt(p.total_rows)} rows</> : 'no data yet'}
            </div>
          </span>
        </span>
      }
      right={
        <div className="row">
          {p.has_data && <Link className="btn sm" to={`/p/${p.id}/overview`}>Open dashboard</Link>}
          <button className="btn sm" onClick={onEdit}>Edit</button>
          <button className="btn sm danger" onClick={onDelete} disabled={running}>Delete</button>
        </div>
      }
    >
      <div className="grid cols-2" style={{ alignItems: 'start' }}>
        <div>
          <div
            className={`drop ${over ? 'over' : ''}`}
            onClick={() => !running && inputRef.current?.click()}
            onDragOver={(e) => { e.preventDefault(); setOver(true) }}
            onDragLeave={() => setOver(false)}
            onDrop={(e) => { e.preventDefault(); setOver(false); const f = e.dataTransfer.files[0]; if (f && !running) send(f) }}
          >
            <input ref={inputRef} type="file" accept=".zip,.xml" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) send(f); e.target.value = '' }} />
            {running ? (
              <div className="stack" style={{ gap: 8 }}>
                {uploading !== null ? (
                  <>
                    <div>Uploading… {uploading} %</div>
                    <div className="progress"><div style={{ width: `${uploading}%` }} /></div>
                  </>
                ) : (
                  <>
                    <div>Importing <b>{st?.filename}</b> · {st?.elapsed}</div>
                    <div className="small">
                      {fmtInt(st?.progress.records ?? 0)} records · {st?.progress.workouts ?? 0} workouts · {st?.progress.routes ?? 0} routes · {st?.progress.ecgs ?? 0} ECGs · {fmtInt(st?.progress.hrv_beats ?? 0)} HRV beats
                    </div>
                  </>
                )}
              </div>
            ) : (
              <>
                <div style={{ fontSize: 26 }}>📦</div>
                <div><b>Drop export.zip here</b> or click to choose</div>
                <div className="small">Re-importing is safe: existing rows are skipped, new ones added.</div>
              </>
            )}
          </div>
          {uploadErr && <div className="err small" style={{ marginTop: 8 }}>{uploadErr}</div>}
          {st?.status === 'failed' && !running && <div className="err small" style={{ marginTop: 8 }}>Last import failed: {st.error}</div>}
          {st?.status === 'completed' && !running && st.result && (
            <div className="small muted" style={{ marginTop: 8 }}>
              Last import: {fmtInt(st.result.records)} records, {st.result.workouts} workouts, {st.result.routes} routes, {st.result.ecgs} ECGs, {st.result.activity_days} activity days
              {st.result.errors > 0 && <span className="err"> · {st.result.errors} warnings</span>}
              {st.profile_updated && ' · profile filled from export'}
            </div>
          )}
        </div>
        <div>
          <h3 style={{ marginBottom: 6 }}>Import history</h3>
          {imports.data && imports.data.length === 0 && <div className="muted small">No imports yet.</div>}
          {imports.data && imports.data.length > 0 && (
            <div className="table-wrap" style={{ maxHeight: 220, overflowY: 'auto' }}>
              <table className="data">
                <thead><tr><th>When</th><th>File</th><th>Status</th><th className="num">Records</th><th>Export date</th></tr></thead>
                <tbody>
                  {imports.data.map((im) => (
                    <tr key={im.id} title={im.error || undefined}>
                      <td>{fmtDateTime(im.started_at)}</td>
                      <td>{im.filename} <span className="faint">{im.size_bytes ? fmtBytes(im.size_bytes) : ''}</span></td>
                      <td><span className={`badge ${im.status === 'completed' ? 'good' : im.status === 'failed' ? 'bad' : ''}`}>{im.status}</span>{im.errors > 0 && <span className="faint small"> {im.errors} warn</span>}</td>
                      <td className="num">{fmtInt(im.records)}</td>
                      <td className="muted">{im.export_date ? fmtDate(im.export_date) : ''}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </Section>
  )
}

function PersonDialog({ initial, onClose, onSave, busy, error }: { initial: Partial<Person>; onClose: () => void; onSave: (p: Partial<Person>) => Promise<void>; busy: boolean; error: unknown }) {
  const [p, setP] = useState<Partial<Person>>({ name: '', emoji: '🙂', color: COLORS[0], dob: '', sex: '', ...initial })
  const set = (k: keyof Person, v: string) => setP((x) => ({ ...x, [k]: v }))
  return (
    <Dialog title={initial.id ? `Edit ${initial.name}` : 'Add person'} onClose={onClose}>
      <form onSubmit={(e) => { e.preventDefault(); onSave({ name: p.name, emoji: p.emoji, color: p.color, dob: p.dob, sex: p.sex }) }}>
        <div className="field"><label>Name</label><input type="text" value={p.name ?? ''} onChange={(e) => set('name', e.target.value)} required autoFocus /></div>
        <div className="field"><label>Avatar</label><div className="emoji-grid">{EMOJIS.map((e) => <button type="button" key={e} className={p.emoji === e ? 'active' : ''} onClick={() => set('emoji', e)}>{e}</button>)}</div></div>
        <div className="field"><label>Colour</label><div className="swatches">{COLORS.map((c) => <button type="button" key={c} className={p.color === c ? 'active' : ''} style={{ background: c }} onClick={() => set('color', c)} aria-label={c} />)}</div></div>
        <div className="grid cols-2">
          <div className="field"><label>Date of birth <span className="faint">(filled from export if empty)</span></label><input type="date" value={p.dob ?? ''} onChange={(e) => set('dob', e.target.value)} /></div>
          <div className="field"><label>Sex</label><select value={p.sex ?? ''} onChange={(e) => set('sex', e.target.value)}><option value="">–</option><option value="female">female</option><option value="male">male</option><option value="other">other</option></select></div>
        </div>
        {error ? <ErrorNote error={error} /> : null}
        <div className="row" style={{ justifyContent: 'flex-end', marginTop: 8 }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn primary" disabled={busy || !p.name?.trim()}>{initial.id ? 'Save' : 'Create'}</button>
        </div>
      </form>
    </Dialog>
  )
}
