// VoiceOver announcements for moments that have no toast or live text of their own (the Swift app's UIAnnounce.say): "Logged
// for today", "Catching up, 2 of 5, …", "All caught up", "Conflict settled", lines carried over. Two hidden live regions in the
// window, made on first use: polite for news, assertive for a problem.
export function announce(text: string, assertive = false): void {
    const id = assertive ? 'announce-assertive' : 'announce-polite';
    let el = document.getElementById(id);
    const made = !el;
    if (!el) {
        el = document.createElement('div');
        el.id = id;
        el.className = 'sr-only';
        el.setAttribute('aria-live', assertive ? 'assertive' : 'polite');
        document.body.appendChild(el);
    }
    const region = el;
    region.textContent = '';
    // A change is what a screen reader reads, even for the same words twice; a region made just now needs a moment to be seen.
    window.setTimeout(() => { region.textContent = text; }, made ? 150 : 50);
}
