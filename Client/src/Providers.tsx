import React from 'react';
import { AuthProvider } from './context/AuthContext';
import { CastleFocusProvider } from './context/CastleFocusContext';
import { CastleResourceProvider } from './dashboard/context/CastleResourceContext';
import { EquipmentProvider } from './equipment/context/EquipmentContext';
import { ResourceProvider } from './currency/context/ResourceContext';
import { ThemeProvider } from './context/ThemeContext';
import { TroopPickerProvider } from './components/TroopPickerModal';
import { MetadataProvider } from './context/MetadataContext';
import SharedSvgDefs from './components/SharedSvgDefs';

export const Providers: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    return (
        <AuthProvider>
            <CastleFocusProvider>
                <ThemeProvider>
                    <CastleResourceProvider>
                        <ResourceProvider>
                            <MetadataProvider>
                                <EquipmentProvider>
                                    <TroopPickerProvider>
                                        <SharedSvgDefs />
                                        {children}
                                    </TroopPickerProvider>
                                </EquipmentProvider>
                            </MetadataProvider>
                        </ResourceProvider>
                    </CastleResourceProvider>
                </ThemeProvider>
            </CastleFocusProvider>
        </AuthProvider>
    );
};
