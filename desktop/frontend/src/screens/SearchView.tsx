// Search results for the sidebar's query: one hit per section, newest day first, match marked.
import {useEffect, useState} from 'react';
import {Display, Row} from '@personal/ui/react';
import {Notes, plural, shortDay, type Hit} from '../api';

export function SearchView({query, onOpen}: {query: string; onOpen: (day: string) => void}) {
    const [hits, setHits] = useState<Hit[] | null>(null);
    useEffect(() => {
        let live = true;
        const t = setTimeout(() => {
            Notes.Search(query).then(h => live && setHits(h ?? []), () => live && setHits([]));
        }, 150); // latest query only
        return () => { live = false; clearTimeout(t); };
    }, [query]);

    const days = hits ? new Set(hits.map(h => h.day)).size : 0;
    return (
        <section aria-label="Search results">
            <header className="head">
                <div className="head-title">
                    <p className="label">Search</p>
                    <Display lines={[query]} size="sm"/>
                </div>
            </header>
            {hits === null ? <p className="loading">Searching…</p> : hits.length === 0 ? (
                <p className="lede muted" style={{marginTop: 'var(--s6)'}}>No pages match “{query}”. Try a shorter word.</p>
            ) : <>
                <p className="small muted" style={{marginTop: 'var(--s5)'}}>{plural(hits.length, 'match')} on {plural(days, 'day')}</p>
                <ul className="list rule-top" style={{marginTop: 'var(--s3)'}}>
                    {hits.map((h, i) => (
                        <li key={h.day + i}>
                            <Row onClick={() => onOpen(h.day)} title={shortDay(h.day)} end={h.heading ?? ''}
                                 meta={<span className="snippet">{h.before}<mark>{h.match}</mark>{h.after}</span>}/>
                        </li>
                    ))}
                </ul>
            </>}
        </section>
    );
}
