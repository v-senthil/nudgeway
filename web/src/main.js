import { jsx as _jsx } from "react/jsx-runtime";
import React from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';
import './styles.css';
import { router } from './router';
const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            retry: false,
            refetchOnWindowFocus: false,
        },
    },
});
const container = document.getElementById('root');
if (!container)
    throw new Error('#root not found');
createRoot(container).render(_jsx(React.StrictMode, { children: _jsx(QueryClientProvider, { client: queryClient, children: _jsx(RouterProvider, { router: router }) }) }));
