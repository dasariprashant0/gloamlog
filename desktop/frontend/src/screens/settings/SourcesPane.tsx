import {useEffect, useState} from 'react';
import {Banner, Button, Dialog, Row, Setting, Switch} from '@personal/ui/react';
import {Notes} from '../../api';
import type {SourceProject} from '../../../bindings/github.com/dasariprashant0/gloamlog/internal/sources';
import {Block, errText, HeadingInput, Panel, Soon, tilde, type PaneProps} from './shared';

type Kind = 'claude' | 'git';
const SOURCE_NAMES: Record<string, string> = {claude: 'Claude Code', git: 'Git'};

// Run on this Mac by the Claude Code Stop hook, only while Gloamlog is open.
const HOOK = '"hooks": { "Stop": [ { "hooks": [ { "type": "command", "command": "pgrep -x Gloamlog >/dev/null && open -g gloamlog://refresh || true" } ] } ] }';

export function SourcesPane({s, set, info}: PaneProps) {
    const [consentFor, setConsentFor] = useState<Kind | null>(null);
    const [note, setNote] = useState('');
    const p = s.sources;
    const anyOn = p.claude || p.git;
    const on = (v: boolean) => <span className="muted small">{v ? 'On' : 'Off'}</span>;
    const patch = (x: Partial<typeof p>) => set(cur => ({sources: {...cur.sources, ...x}}));

    // The project list: Claude Code's projects and the repositories under the Git folders. Looked up again when those change.
    const [projects, setProjects] = useState<SourceProject[] | 'loading' | null>(null);
    const rootsKey = p.gitRoots.join('\n');
    useEffect(() => {
        if (!anyOn) return;
        let live = true;
        setProjects('loading');
        Notes.SourceProjects().then(list => { if (live) setProjects(list ?? []); }).catch(e => { if (live) { setProjects(null); setNote(`Couldn't list the projects: ${errText(e)}`); } });
        return () => { live = false; };
    }, [anyOn, p.claude, p.git, rootsKey]);
    // allowed holds the ticked roots, or with "Every session and tab" the skipped ones. Roots compare without case, like the readers.
    const listed = (root: string) => p.allowed.some(a => a.toLowerCase() === root.toLowerCase());
    const isOn = (root: string) => listed(root) !== p.allProjects;
    const setProject = (root: string, v: boolean) => set(cur => {
        const rest = cur.sources.allowed.filter(a => a.toLowerCase() !== root.toLowerCase());
        return {sources: {...cur.sources, allowed: v !== cur.sources.allProjects ? [...rest, root] : rest}};
    });

    // Nothing is switched on before the consent sheet is accepted.
    const setSource = (kind: Kind, v: boolean) => (v && !p.consented ? setConsentFor(kind) : patch({[kind]: v}));
    const accept = () => { if (consentFor) patch({consented: true, [consentFor]: true}); setConsentFor(null); };

    async function copyHook() {
        try { await navigator.clipboard.writeText(HOOK); setNote('Copied. Paste it into ~/.claude/settings.json.'); }
        catch (e) { setNote(`Couldn't copy: ${errText(e)}`); }
    }
    async function addGitFolder() {
        try {
            const path = await Notes.ChooseFolder('Add a repository, or a folder that holds your repositories.');
            if (path && !p.gitRoots.includes(path)) patch({gitRoots: [...p.gitRoots, path]});
        } catch (e) { setNote(`Couldn't add the folder: ${errText(e)}`); }
    }

    return (
        <Panel>
            <Block title="Fill in my log from my work">
                <Banner tone="accent">Read-only, on this Mac only, nothing is sent anywhere, secrets are redacted.</Banner>
                <Setting title="Claude Code" hint="Reads the session files Claude Code keeps in ~/.claude, only for the projects you tick below: the first line of your prompts, the files it edited, commits and pull-request links.">
                    <Switch checked={p.claude} onChange={v => setSource('claude', v)} label={on(p.claude)}/>
                </Setting>
                <Setting title="Git" hint="Runs git log in the repositories you tick and lists only commits made with your own git email. It never fetches, pushes or changes anything.">
                    <Switch checked={p.git} onChange={v => setSource('git', v)} label={on(p.git)}/>
                </Setting>
                <Setting title="Every session and tab" hint="Reads all your Claude Code sessions, in every folder and tab, without ticking projects. Use the list below to skip a project.">
                    <Switch checked={p.allProjects} onChange={v => patch({allProjects: v, allowed: []})} label={on(p.allProjects)}/>
                </Setting>
                <Setting title="Claude writes my log" hint="Lets Claude Code add a short entry (the problem, what it did, what is left) to today's page when a piece of work is done. Registers Gloamlog's helper with Claude Code and adds a short section to ~/.claude/CLAUDE.md.">
                    <div className="row-gap"><Button variant="line" size="sm" disabled>Connect Claude Code</Button><Soon/></div>
                </Setting>
                <Setting title="Add commits for me" hint="Writes today's commits into a list page by themselves, at their commit time, in the Work or Personal list their folder belongs to: when the page opens, about every 5 minutes, and right away when a Claude Code session ends (see the hook below). Edit or delete them like any other line. A past day has an Add this day's commits button.">
                    <Switch checked={p.autoAdd} onChange={v => patch({autoAdd: v})} label={on(p.autoAdd)}/>
                </Setting>
                <Setting title="Instant update" hint={note || 'Optional. Add this hook to ~/.claude/settings.json so Gloamlog updates the moment a Claude Code session ends, instead of waiting up to 5 minutes.'}>
                    <Button variant="line" size="sm" onClick={copyHook}>Copy hook</Button>
                </Setting>
                <Setting title="Insert under" hint="The heading the lines you choose are added under. It is added at the end of the page when the page has none.">
                    <HeadingInput value={p.heading} placeholder="What I did" label="Insert under this heading" onCommit={heading => patch({heading})}/>
                </Setting>
            </Block>

            {p.git && (
                <Block title="Git folders" lede="Add a repository, or a folder that holds your repositories (for example ~/Projects). Gloamlog looks up to three levels down; you tick the ones to include below.">
                    {p.gitRoots.length > 0 && (
                        <ul className="list">
                            {p.gitRoots.map(r => (
                                <li key={r}><Row title={tilde(r, info)} end={<Button variant="ghost" size="sm" onClick={() => patch({gitRoots: p.gitRoots.filter(x => x !== r)})}>Remove</Button>}/></li>
                            ))}
                        </ul>
                    )}
                    <div className="row-gap" style={{marginTop: 'var(--s3)'}}><Button variant="line" size="sm" onClick={addGitFolder}>Add folder…</Button></div>
                </Block>
            )}

            <Block title="Projects">
                {!anyOn
                    ? <p className="small muted">Turn on Claude Code or Git to choose projects.</p>
                    : projects === 'loading' ? <p className="small muted" role="status">Looking for projects…</p>
                    : projects === null ? <p className="small muted">{note}</p>
                    : projects.length === 0 ? <p className="small muted">No projects found yet. Claude Code's appear after its first session{p.git ? '; add a Git folder above for repositories' : ''}.</p>
                    : (
                        <ul className="list">
                            {projects.map(pr => {
                                const when = new Date(pr.lastActive);
                                const meta = [tilde(pr.root, info), pr.sources.map(x => SOURCE_NAMES[x] ?? x).join(', '),
                                    when.getFullYear() > 1 ? `active ${when.toLocaleDateString('en-GB', {day: 'numeric', month: 'short'})}` : ''].filter(Boolean).join(' · ');
                                const v = isOn(pr.root);
                                return (
                                    <li key={pr.root}>
                                        <Row title={pr.name} meta={meta} end={<Switch checked={v} onChange={x => setProject(pr.root, x)}
                                            label={<><span className="sr-only">{pr.name}: </span><span className="muted small">{v ? 'On' : 'Off'}</span></>}/>}/>
                                    </li>
                                );
                            })}
                        </ul>
                    )}
                <p className="small muted" style={{marginTop: 'var(--s3)'}}>{p.allProjects
                    ? 'Every project is read except the ones you switch off here.'
                    : 'Hidden projects are skipped before any text is read. Nothing is included until you tick it.'} Gloamlog stores nothing from what it reads: it looks again when you open a day.</p>
            </Block>

            <Dialog open={consentFor !== null} onClose={() => setConsentFor(null)} title="Let Gloamlog read your Claude Code and Git activity?"
                actions={<><Button variant="line" onClick={() => setConsentFor(null)}>Not now</Button><Button onClick={accept}>Turn on</Button></>}>
                <div className="stack">
                    <p>Read-only. It stays on this Mac and nothing is sent anywhere. Unless you switch on "Every session and tab", projects you have not ticked are skipped before any text is read.</p>
                    <p><strong>Claude Code.</strong> The session files in ~/.claude/projects (and $CLAUDE_CONFIG_DIR when it is set). To list your projects it reads the working folder named at the start of each file. For a project you tick it reads the whole file and keeps only: the first line of the prompts you typed, the names of the files Claude edited, git commit messages from its commands, pull-request links from a pull-request command, and when each record was written. Claude's replies, other tool output and file contents are not kept.</p>
                    <p><strong>Git.</strong> In the folders you add it looks for repositories, up to three levels down. For the repositories you tick it runs git log and git config user.email to find your own commits: hash, subject and time. It never fetches, pushes or changes a repository.</p>
                    <p><strong>Before anything is shown.</strong> Tokens, keys, emails and long secrets are hidden, and your home folder is shown as ~. Nothing is saved: Gloamlog reads again when you open a day. The only thing it writes is lines into your page: the ones you choose to insert, or all of today's when you switch on "Add them for me".</p>
                </div>
            </Dialog>
        </Panel>
    );
}
