package GameCommands

// DefaultProductionSplLIDs lists **spl** LIDs the dashboard polls for production queues.
//
// Verified from captures: 0 = barracks/recruit, 1 = siege/defense workshop (tool crafting).
// 2–5 are the usual ordering for refinery, toolsmith, DragonHoard, DragonBreathForge in HTML5 builds—
// confirm with game_websocket.log while each building’s queue panel is open and adjust if needed.
var DefaultProductionSplLIDs = []int{0, 1, 2, 3, 4, 5}
