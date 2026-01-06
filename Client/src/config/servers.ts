// Server configuration for game login
// Keys are the full world names as they appear in the game's world selector
// These names are typed into the search box during automated login

export const SERVER_OPTIONS: Record<string, string> = {
    'United States': 'ep-live-us1-game',
    'World: 2': 'ep-live-world2-game',
    'World: 3': 'ep-live-world3-game',
    'World: 4': 'ep-live-world4-game',
    // Add more servers as needed
};

// Get display name from server ID
export const getServerDisplayName = (serverId: string): string => {
    const entry = Object.entries(SERVER_OPTIONS).find(([_, id]) => id === serverId);
    return entry ? entry[0] : 'United States';
};

// Default server
export const DEFAULT_SERVER = 'United States';
