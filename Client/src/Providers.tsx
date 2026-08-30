import React from 'react';
import { AuthProvider } from './context/AuthContext';
import { CastleFocusProvider } from './context/CastleFocusContext';
import { ThemeProvider } from './context/ThemeContext';
import { TroopPickerProvider } from './components/TroopPickerModal';
import { ToolPickerProvider } from './components/ToolPickerModal';
import { TCIPickerProvider } from './components/TCIPickerModal';
import { RiftMapProvider } from './Rift/context/RiftMapContext';
import { MovementProvider } from './Movement/context/MovementContext';
import { MetadataProvider } from './context/MetadataContext';
import SharedSvgDefs from './components/SharedSvgDefs';
import { APIProvider } from './api/ApiContext';

export const Providers: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    return (
        <APIProvider>
			<MetadataProvider>
				<AuthProvider>
					<CastleFocusProvider>
							<ThemeProvider>
                                    <RiftMapProvider>
                                        <MovementProvider>
                                            <TroopPickerProvider>
                                                    <ToolPickerProvider>
                                                        <TCIPickerProvider>
                                                            <SharedSvgDefs />
                                                            {children}
                                                        </TCIPickerProvider>
                                                    </ToolPickerProvider>
                                            </TroopPickerProvider>
                                        </MovementProvider>
                                    </RiftMapProvider>
							</ThemeProvider>
					</CastleFocusProvider>
				</AuthProvider>
			</MetadataProvider>
        </APIProvider>
    );
};
