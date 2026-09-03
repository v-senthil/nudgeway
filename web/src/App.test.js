import { jsx as _jsx } from "react/jsx-runtime";
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { App } from './App';
describe('App', () => {
    beforeEach(() => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
            ok: true,
            json: () => Promise.resolve({ status: 'ok' }),
        }));
    });
    afterEach(() => vi.unstubAllGlobals());
    it('renders the header', () => {
        render(_jsx(App, {}));
        expect(screen.getByText('fullWA')).toBeInTheDocument();
    });
});
