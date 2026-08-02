import { useState } from 'react'
import { Flag, X } from 'lucide-react'
import { submitReport, type ReportTargetType } from './api/client'
import './report-dialog.css'

export type ReportTarget = {
  type: ReportTargetType
  id: string
  label: string
}

const REPORT_REASONS = ['垃圾广告', '违规内容', '侵权', '其他']

const DETAIL_MAX = 1000

// ReportDialog 轻量举报弹窗：选择预设原因并可附加说明，提交到 /api/v1/reports。
// 登录校验由调用方在打开前完成，这里只负责收集与提交。
export default function ReportDialog({
  target,
  onClose,
  onSubmitted,
}: {
  target: ReportTarget
  onClose: () => void
  onSubmitted: (message: string) => void
}) {
  const [reason, setReason] = useState(REPORT_REASONS[0])
  const [detail, setDetail] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const targetName = target.type === 'project' ? '项目' : '评论'

  const submit = async () => {
    setSubmitting(true)
    setError('')
    try {
      await submitReport({
        target_type: target.type,
        target_id: target.id,
        reason,
        detail: detail.trim() || undefined,
      })
      onSubmitted('举报已提交，我们会尽快处理')
    } catch (value) {
      setError(value instanceof Error ? value.message : '举报提交失败，请稍后重试')
      setSubmitting(false)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="report-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={`举报${targetName}`}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <button className="modal-close" onClick={onClose} aria-label="关闭"><X size={18} /></button>
        <h2><Flag size={18} /> 举报{targetName}</h2>
        <p className="report-target" title={target.label}>{target.label}</p>
        <fieldset className="report-reasons">
          <legend>选择举报原因</legend>
          {REPORT_REASONS.map((item) => (
            <label key={item} className={reason === item ? 'active' : ''}>
              <input
                type="radio"
                name="report-reason"
                value={item}
                checked={reason === item}
                onChange={() => setReason(item)}
              />
              {item}
            </label>
          ))}
        </fieldset>
        <textarea
          className="report-detail"
          maxLength={DETAIL_MAX}
          placeholder="补充说明（选填）"
          value={detail}
          onChange={(event) => setDetail(event.target.value)}
        />
        {error && <div className="report-error">{error}</div>}
        <div className="report-actions">
          <button className="report-cancel" onClick={onClose} disabled={submitting}>取消</button>
          <button className="report-submit" onClick={() => void submit()} disabled={submitting}>
            {submitting ? '提交中…' : '提交举报'}
          </button>
        </div>
      </div>
    </div>
  )
}
