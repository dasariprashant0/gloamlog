export function Jot() {
    return (
        <div className="jot">
            <p className="label">Jot to today</p>
            <textarea className="textarea" autoFocus placeholder="A thought, a link, a done thing…"/>
            <p className="small muted">Return to save · Esc to close</p>
        </div>
    );
}
