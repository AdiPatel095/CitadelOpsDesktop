import { API_CONFIG } from '../config/api';

export interface License {
    hardwareID: string;
    credits: number;
    createdAt?: string;
    updatedAt?: string;
}

// Base URL for the API
const BASE_URL = API_CONFIG.BASE_URL;

// Common fetch options
const commonOptions: RequestInit = {
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
};

export interface ReconfigureResponse {
    success: boolean;
    message?: string;
    newCredits?: number;
}

export const LicenseService = {
    /**
     * Fetches license information by hardware ID from the cloud database.
     * This endpoint does not require authentication.
     */
    getLicenseByHardwareID: async (hardwareID: string): Promise<License | null> => {
        try {
            const response = await fetch(
                `${BASE_URL}${API_CONFIG.ENDPOINTS.LICENSE.GET_BY_HARDWARE_ID}/${hardwareID}`,
                {
                    ...commonOptions,
                    method: 'GET',
                }
            );

            if (!response.ok) {
                if (response.status === 404) {
                    // License not found for this hardware ID
                    return null;
                }
                console.error('Failed to fetch license:', response.statusText);
                return null;
            }

            return await response.json();
        } catch (error) {
            console.error('Error fetching license by hardware ID:', error);
            return null;
        }
    },

    /**
     * Sends stat priorities to the backend to reconfigure the loadout.
     * Costs 10,000 credits.
     */
    reconfigureLoadout: async (
        hardwareID: string,
        equipmentMode: 'Commander' | 'Castellan',
        priorityStats: string[]
    ): Promise<ReconfigureResponse> => {
        try {
            const response = await fetch(
                `${BASE_URL}${API_CONFIG.ENDPOINTS.EQUIPMENT.RECONFIGURE_LOADOUT}`,
                {
                    ...commonOptions,
                    method: 'POST',
                    body: JSON.stringify({
                        hardwareID,
                        equipmentMode,
                        priorityStats,
                    }),
                }
            );

            if (!response.ok) {
                const errorData = await response.json().catch(() => ({}));
                return {
                    success: false,
                    message: errorData.message || 'Failed to reconfigure loadout',
                };
            }

            return await response.json();
        } catch (error) {
            console.error('Error reconfiguring loadout:', error);
            return {
                success: false,
                message: 'Network error. Please try again.',
            };
        }
    },
};
