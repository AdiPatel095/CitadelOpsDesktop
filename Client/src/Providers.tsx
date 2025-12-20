import React from 'react';
import { AuthProvider } from './context/AuthContext';
import { CastleResourceProvider } from './dashboard/context/CastleResourceContext';
import { EquipmentProvider } from './equipment/context/EquipmentContext';
import { ResourceProvider } from './currency/context/ResourceContext';
import { ThemeProvider } from './context/ThemeContext';

export const Providers: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    return (
        <AuthProvider>
            <ThemeProvider>
                <CastleResourceProvider>
                    <ResourceProvider>
                        <EquipmentProvider>
                            {children}
                        </EquipmentProvider>
                    </ResourceProvider>
                </CastleResourceProvider>
            </ThemeProvider>
        </AuthProvider>
    );
};
