import { useEffect, useState } from 'react'
import {
  Activity,
  BrainCircuit,
  CheckCircle2,
  Clock3,
  RefreshCw,
  Sparkles,
  TriangleAlert,
} from 'lucide-react'

import type { AIAnalysisRecord } from '../types'
import {
  analyzeIncident,
  fetchAIAnalysisHistory,
  fetchLatestAIAnalysis,
} from '../services/app'

type Props = { incidentId: number }

function isNotFound(error: unknown): boolean {
  return error instanceof Error &&
    (error.message.includes('404') || error.message.toLowerCase().includes('analysis not found'))
}

function AIAnalysisPanel({ incidentId }: Props) {
  const [record, setRecord] = useState<AIAnalysisRecord | null>(null)
  const [historyCount, setHistoryCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const latest = await fetchLatestAIAnalysis(incidentId).catch((err: unknown) => {
          if (isNotFound(err)) return null
          throw err
        })
        const history = await fetchAIAnalysisHistory(incidentId).catch(() => null)
        if (!cancelled) {
          setRecord(latest)
          setHistoryCount(history?.count ?? 0)
        }
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Unable to load AI analysis')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [incidentId])

  async function runAnalysis() {
    setRunning(true)
    setError('')
    try {
      const result = await analyzeIncident(incidentId)
      setRecord(result)
      const history = await fetchAIAnalysisHistory(incidentId).catch(() => null)
      setHistoryCount(history?.count ?? 0)
    } catch (err) {
      const fallbackMessage = err instanceof Error ? err.message : 'AI analysis failed'

      // The API persists failed runs in ai_analyses even when the
      // analyze endpoint returns HTTP 500. Reload that record so the
      // operator sees the actionable persisted failure message.
      try {
        const latest = await fetchLatestAIAnalysis(incidentId)
        setRecord(latest)
        const history = await fetchAIAnalysisHistory(incidentId).catch(() => null)
        setHistoryCount(history?.count ?? 0)
        setError(latest.error_message || fallbackMessage)
      } catch {
        setError(fallbackMessage)
      }
    } finally {
      setRunning(false)
    }
  }

  const analysis = record?.analysis

  return (
    <section className="panel detail-section ai-analysis-panel">
      <div className="panel-header ai-panel-header">
        <div>
          <div className="eyebrow">CLOUDFLARE WORKERS AI</div>
          <h3><BrainCircuit size={18} /> AI incident analysis</h3>
          <p>Evidence-grounded investigation powered by Gemma and retrieved knowledge.</p>
        </div>
        <button
          type="button"
          className="filter-button ai-run-button"
          onClick={() => void runAnalysis()}
          disabled={running || loading}
        >
          {running ? <RefreshCw size={15} className="spin" /> : <Sparkles size={15} />}
          {running ? 'Analyzing…' : record ? 'Run again' : 'Analyze incident'}
        </button>
      </div>

      {error && <div className="api-error">{error}</div>}
      {loading && <p className="detail-empty">Loading saved AI analysis…</p>}

      {!loading && !record && !error && (
        <div className="ai-empty-state">
          <BrainCircuit size={26} />
          <strong>No AI analysis yet</strong>
          <span>Run an analysis to inspect likely causes, evidence and recommended actions.</span>
        </div>
      )}

      {!loading && record && (
        <>
          <div className="ai-meta-row">
            <span className={record.status === 'COMPLETED' ? 'ai-status complete' : 'ai-status'}>
              {record.status === 'COMPLETED' ? <CheckCircle2 size={13} /> : <Clock3 size={13} />}
              {record.status}
            </span>
            <span>{record.provider} · {record.model}</span>
            <span>{historyCount} run{historyCount === 1 ? '' : 's'}</span>
          </div>

          {record.error_message && <div className="api-error">{record.error_message}</div>}

          {analysis && (
            <>
              <div className="ai-summary-block">
                <div className="ai-section-label"><Sparkles size={14} /> EXECUTIVE SUMMARY</div>
                <p>{analysis.summary}</p>
              </div>

              <div className="ai-cause-grid">
                <div className="ai-cause-card">
                  <span>LIKELY CAUSE</span>
                  <strong>{analysis.likely_cause}</strong>
                </div>
                <div className="ai-cause-card">
                  <span>CAUSE CONFIDENCE</span>
                  <strong>{Math.round(analysis.confidence * 100)}%</strong>
                  <small>{analysis.cause_status}</small>
                </div>
              </div>

              <div className="ai-subsection">
                <h4><Activity size={15} /> Evidence references</h4>
                {analysis.evidence.length === 0 ? (
                  <p className="detail-empty">No evidence references supplied.</p>
                ) : (
                  <div className="ai-evidence-list">
                    {analysis.evidence.map((item, index) => (
                      <article className="ai-evidence-item" key={`${item.source_type}-${item.source_id}-${index}`}>
                        <span>{item.source_type} · {item.source_id}</span>
                        <p>{item.claim}</p>
                      </article>
                    ))}
                  </div>
                )}
              </div>

              <div className="ai-subsection">
                <h4><CheckCircle2 size={15} /> Suggested response</h4>
                {analysis.recommended_actions.length === 0 ? (
                  <p className="detail-empty">No recommended actions returned.</p>
                ) : (
                  <div className="ai-recommendation-list">
                    {analysis.recommended_actions.map((action, index) => (
                      <article className="ai-recommendation-item" key={`${action.action_type}-${index}`}>
                        <div>
                          <strong>{action.description}</strong>
                          <span>{action.action_type}</span>
                        </div>
                        <small>{action.requires_approval ? 'Approval required' : 'No approval required'}</small>
                      </article>
                    ))}
                  </div>
                )}
              </div>

              <div className="ai-subsection">
                <h4><TriangleAlert size={15} /> Risks</h4>
                {analysis.risks.length === 0 ? (
                  <p className="detail-empty">No risks reported.</p>
                ) : (
                  <ul className="ai-risk-list">
                    {analysis.risks.map((risk, index) => <li key={index}>{risk}</li>)}
                  </ul>
                )}
              </div>
            </>
          )}
        </>
      )}
    </section>
  )
}

export default AIAnalysisPanel
