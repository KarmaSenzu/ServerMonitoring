export default function EmptyState({ icon, title, description, action }) {
  return (
    <div className="empty-state glass">
      {icon}
      <h3>{title}</h3>
      {description && <p>{description}</p>}
      {action && <div className="empty-state-action">{action}</div>}
    </div>
  )
}
