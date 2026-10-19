import {test} from 'node:test';
import assert from 'node:assert/strict';
import * as M from './model.ts';

const L = (text: string, more: Partial<M.Line> = {}): M.Line => ({time: '', text, mark: '', task: false, other: false, blank: 0, raw: `- ${text}`, ...more});
const texts = (s: M.State) => s.lines.map(l => l.text);

test('cutMark takes only a trailing #next / #blocked, any case', () => {
    assert.deepEqual(M.cutMark('write the test #NEXT'), ['write the test', 'next']);
    assert.deepEqual(M.cutMark('#blocked'), ['', 'blocked']);
    assert.deepEqual(M.cutMark('tagged #next in the middle'), ['tagged #next in the middle', null]);
});

test('clock is HH:MM', () => {
    assert.equal(M.clock(new Date(2026, 9, 18, 9, 5)), '09:05');
});

test('commit writes the draft, cuts a mark, and keeps no history when nothing changed', () => {
    let s = M.focus(M.fresh([L('a')]), 0);
    assert.equal(M.commit(s).past.length, 0);
    s = M.setDraft(s, 'a better #next');
    const c = M.commit(s);
    assert.deepEqual([c.lines[0].text, c.lines[0].mark, c.edit, c.past.length], ['a better', 'next', -1, 1]);
    const plain = M.commit(M.setDraft(M.focus(c, 0), 'a better'));
    assert.equal(plain.lines[0].mark, '', 'deleting the word makes it done');
});

test('a task line keeps its box mark unless the draft says otherwise', () => {
    const task = L('call Sam', {task: true, mark: 'next'});
    let s = M.focus(M.fresh([task]), 0);
    assert.equal(s.draft, 'call Sam');
    assert.equal(M.commit(M.setDraft(s, 'call Sam today')).lines[0].mark, 'next');
    s = M.setMark(s, 0, '');
    assert.equal(M.commit(s).lines[0].mark, '', '⌘1 ticks it');
});

test('Enter starts a timed line after the current one; Enter on an empty new line does nothing', () => {
    let s = M.newLineAfter(M.focus(M.fresh([L('a'), L('b')]), 0), 0, '10:30');
    assert.deepEqual([texts(s), s.edit, s.lines[1].time], [['a', '', 'b'], 1, '10:30']);
    assert.equal(M.newLineAfter(s, 1, '10:31'), s);
    s = M.focus(s, 2);
    assert.deepEqual(texts(s), ['a', 'b'], 'leaving an empty new line removes it');
    assert.equal(s.edit, 1, 'and the target index follows');
});

test('the first line of an empty list gets a blank line after the heading', () => {
    const s = M.append(M.fresh([]), 'first', '09:00');
    assert.equal(s.lines[0].blank, 1);
    assert.equal(M.append(s, 'second', '09:01').lines[1].blank, 0);
});

test('append reads a trailing mark from the text, or takes one', () => {
    assert.equal(M.append(M.fresh([]), 'x #blocked', '09:00').lines[0].mark, 'blocked');
    assert.equal(M.append(M.fresh([]), 'x', '09:00', 'next').lines[0].mark, 'next');
    assert.equal(M.append(M.fresh([]), '   ', '09:00').lines.length, 0);
});

test('setMark while editing changes the draft too; on another line just the mark', () => {
    let s = M.setMark(M.focus(M.fresh([L('a'), L('b')]), 0), 0, 'blocked');
    assert.deepEqual([s.lines[0].mark, s.draft, s.edit], ['blocked', 'a #blocked', 0]);
    s = M.setMark(s, 1, 'next');
    assert.equal(s.lines[1].mark, 'next');
});

test('move skips Other blocks; removeLine edits the line above and keeps the gap after the heading', () => {
    const other = L('A paragraph', {other: true, raw: 'A paragraph'});
    let s = M.focus(M.fresh([L('a', {blank: 1}), other, L('b')]), 0);
    s = M.move(s, 1);
    assert.equal(s.edit, 2);
    assert.equal(M.focus(s, 1).edit, -1, 'an Other block is not edited here');
    const r = M.removeLine(M.focus(M.fresh([L('a', {blank: 1}), L('b')]), 0), 0);
    assert.deepEqual([texts(r), r.lines[0].blank, r.edit], [['b'], 1, -1]);
});

test('undo and redo work across lines, and undo while typing takes the typing back', () => {
    let s = M.append(M.append(M.fresh([]), 'one', '09:00'), 'two', '09:01');
    s = M.undo(s);
    assert.deepEqual(texts(s), ['one']);
    s = M.redo(s);
    assert.deepEqual(texts(s), ['one', 'two']);
    s = M.undo(M.setDraft(M.focus(s, 0), 'changed'));
    assert.deepEqual(texts(s), ['one', 'two']);
});

test('undo never brings back an empty new line', () => {
    let s = M.newLineAfter(M.focus(M.fresh([L('a', {blank: 1})]), 0), 0, '10:00');
    s = M.undo(M.removeLine(s, 1));
    assert.deepEqual(texts(s), ['a']);
});

test('setTime moves a line before the first later time; untimed lines are passed over; bad times are ignored', () => {
    const s = M.fresh([L('a', {time: '09:00', blank: 1}), L('untimed'), L('c', {time: '11:00'})]);
    const t = M.setTime(s, 2, '08:30');
    assert.deepEqual([texts(t), t.lines[0].blank, t.lines[1].blank], [['c', 'a', 'untimed'], 1, 0]);
    assert.equal(M.setTime(s, 0, '25:00'), s);
    const u = M.setTime(s, 0, '12:00');
    assert.deepEqual([texts(u), u.lines[0].blank, u.lines[2].blank], [['untimed', 'c', 'a'], 1, 0]);
});

test('withDraft drops an empty new line; renderIndex follows it', () => {
    const s = M.newLineAfter(M.focus(M.fresh([L('a'), L('b')]), 0), 0, '10:00');
    assert.deepEqual(M.withDraft(s).map(l => l.text), ['a', 'b']);
    assert.deepEqual([0, 1, 2].map(i => M.renderIndex(s, i)), [0, -1, 1]);
    assert.equal(M.renderIndex(M.fresh([L('a')]), 0), 0);
});

test('adopt takes lines from Go (an appended jot) as one undoable step', () => {
    const s = M.adopt(M.focus(M.fresh([L('a')]), 0), [L('a'), L('jot')]);
    assert.deepEqual([texts(s), s.edit], [['a', 'jot'], -1]);
    assert.deepEqual(texts(M.undo(s)), ['a']);
});

test('summary counts real lines, next and blocked; Other blocks do not count', () => {
    assert.equal(M.summary([]), 'Nothing yet');
    assert.equal(M.summary([L('a'), L('b', {mark: 'next'}), L('c', {mark: 'blocked'}), L('p', {other: true})]), '3 lines, 1 next, 1 blocked');
    assert.equal(M.summary([L('a')]), '1 line');
});

test('standup groups lines by mark in file order; other blocks and empty lines are left out', () => {
    const lines = [L('a'), L('b', {mark: 'next'}), L('a note', {other: true}), L(''), L('c', {mark: 'blocked'}),
        L('call Sam', {task: true, mark: 'next'}), L('d')];
    assert.deepEqual(M.standup(lines), {done: [0, 6], next: [1, 5], blocked: [4]});
});

test('standupText is the spec example: three titled groups, times dropped', () => {
    const lines = [
        L('Fixed the login redirect loop', {time: '09:40'}),
        L('Waiting on API keys from ops', {time: '15:30', mark: 'blocked'}),
        L('Write the migration test', {time: '17:45', mark: 'next'}),
    ];
    assert.equal(M.standupText(lines),
        'Done\n- Fixed the login redirect loop\nNext\n- Write the migration test\nBlocked\n- Waiting on API keys from ops');
});

test('standupText leaves out empty groups and images, keeps markdown, and indents continuation rows', () => {
    const lines = [L('Shipped **CSV export** `1a2b3c4` ![](assets/a.png)\nNeeds a note'), L('![](assets/b.png)'), L('x', {other: true})];
    assert.equal(M.standupText(lines), 'Done\n- Shipped **CSV export** `1a2b3c4`\n  Needs a note');
    assert.equal(M.standupText([]), '');
});
