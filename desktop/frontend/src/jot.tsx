import {StrictMode} from 'react';
import {createRoot} from 'react-dom/client';
import '@personal/ui/tokens.css';
import '@personal/ui/base.css';
import '@personal/ui/components.css';
import './brand.css';
import './app.css';
import {Jot} from './screens/Jot';

createRoot(document.getElementById('root')!).render(<StrictMode><Jot/></StrictMode>);
