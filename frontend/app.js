const canvas = document.querySelector("#board");
const context = canvas.getContext("2d");
const startButton = document.querySelector("#start");
const pauseButton = document.querySelector("#pause");
const resetButton = document.querySelector("#reset");
const generationElement = document.querySelector("#generation");

const worldSize = 1000000;
const initialViewSize = 100;
const minCellSize = 0.035;
const maxCellSize = 80;
let cellSize = Math.max(4, Math.min(maxCellSize, window.innerWidth / initialViewSize));
let cameraX = worldSize / 2;
let cameraY = worldSize / 2;
let generation = 0;
let running = false;
let animationFrame;
let dragStart;
const liveCells = new Set();

function resizeCanvas() {
  const pixelRatio = window.devicePixelRatio || 1;
  canvas.width = Math.floor(canvas.clientWidth * pixelRatio);
  canvas.height = Math.floor(canvas.clientHeight * pixelRatio);
  context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
  render();
}

function worldToScreen(column, row) {
  return {
    x: (column - cameraX) * cellSize + canvas.clientWidth / 2,
    y: (row - cameraY) * cellSize + canvas.clientHeight / 2,
  };
}

function screenToWorld(x, y) {
  return {
    column: Math.floor((x - canvas.clientWidth / 2) / cellSize + cameraX),
    row: Math.floor((y - canvas.clientHeight / 2) / cellSize + cameraY),
  };
}

function cellKey(column, row) {
  return `${column},${row}`;
}

function renderGrid() {
  const maxLines = 250;
  const visibleColumns = canvas.clientWidth / cellSize;
  const visibleRows = canvas.clientHeight / cellSize;
  const interval = Math.max(1, Math.ceil(Math.max(visibleColumns, visibleRows) / maxLines));
  const left = Math.max(0, Math.floor(cameraX - visibleColumns / 2));
  const right = Math.min(worldSize, Math.ceil(cameraX + visibleColumns / 2));
  const top = Math.max(0, Math.floor(cameraY - visibleRows / 2));
  const bottom = Math.min(worldSize, Math.ceil(cameraY + visibleRows / 2));

  context.beginPath();
  context.strokeStyle = "rgba(255, 255, 255, 0.12)";
  context.lineWidth = 1;
  for (let column = left - (left % interval); column <= right; column += interval) {
    const x = worldToScreen(column, 0).x + 0.5;
    context.moveTo(x, 0);
    context.lineTo(x, canvas.clientHeight);
  }
  for (let row = top - (top % interval); row <= bottom; row += interval) {
    const y = worldToScreen(0, row).y + 0.5;
    context.moveTo(0, y);
    context.lineTo(canvas.clientWidth, y);
  }
  context.stroke();
}

function render() {
  context.fillStyle = "#000";
  context.fillRect(0, 0, canvas.clientWidth, canvas.clientHeight);
  renderGrid();
  context.fillStyle = "#fff";
  for (const key of liveCells) {
    const [column, row] = key.split(",").map(Number);
    const position = worldToScreen(column, row);
    if (position.x + cellSize < 0 || position.y + cellSize < 0 || position.x > canvas.clientWidth || position.y > canvas.clientHeight) continue;
    context.fillRect(position.x, position.y, Math.max(1, cellSize), Math.max(1, cellSize));
  }
}

function updateGeneration() {
  generationElement.textContent = `Génération : ${generation}`;
}

function toggleCell(column, row) {
  if (column < 0 || column >= worldSize || row < 0 || row >= worldSize) return;
  const key = cellKey(column, row);
  if (liveCells.has(key)) liveCells.delete(key);
  else liveCells.add(key);
  render();
}

function nextGeneration() {
  const neighbors = new Map();
  for (const key of liveCells) {
    const [column, row] = key.split(",").map(Number);
    for (let rowOffset = -1; rowOffset <= 1; rowOffset += 1) {
      for (let columnOffset = -1; columnOffset <= 1; columnOffset += 1) {
        if (columnOffset === 0 && rowOffset === 0) continue;
        const neighborColumn = column + columnOffset;
        const neighborRow = row + rowOffset;
        if (neighborColumn >= 0 && neighborColumn < worldSize && neighborRow >= 0 && neighborRow < worldSize) {
          const neighborKey = cellKey(neighborColumn, neighborRow);
          neighbors.set(neighborKey, (neighbors.get(neighborKey) || 0) + 1);
        }
      }
    }
  }
  const next = new Set();
  for (const [key, count] of neighbors) {
    if (count === 3 || (count === 2 && liveCells.has(key))) next.add(key);
  }
  liveCells.clear();
  for (const key of next) liveCells.add(key);
  generation += 1;
  updateGeneration();
  render();
}

function animate() {
  if (!running) return;
  nextGeneration();
  animationFrame = requestAnimationFrame(animate);
}

canvas.addEventListener("pointerdown", (event) => {
  canvas.setPointerCapture(event.pointerId);
  dragStart = { x: event.clientX, y: event.clientY, cameraX, cameraY };
});

canvas.addEventListener("pointerup", (event) => {
  if (!dragStart) return;
  const moved = Math.hypot(event.clientX - dragStart.x, event.clientY - dragStart.y);
  if (!running && moved < 5) {
    const rectangle = canvas.getBoundingClientRect();
    const position = screenToWorld(event.clientX - rectangle.left, event.clientY - rectangle.top);
    toggleCell(position.column, position.row);
  }
  dragStart = null;
});

canvas.addEventListener("pointermove", (event) => {
  if (!dragStart) return;
  cameraX = Math.max(0, Math.min(worldSize, dragStart.cameraX - (event.clientX - dragStart.x) / cellSize));
  cameraY = Math.max(0, Math.min(worldSize, dragStart.cameraY - (event.clientY - dragStart.y) / cellSize));
  render();
});

canvas.addEventListener("wheel", (event) => {
  event.preventDefault();
  const rectangle = canvas.getBoundingClientRect();
  const before = screenToWorld(event.clientX - rectangle.left, event.clientY - rectangle.top);
  cellSize = Math.max(minCellSize, Math.min(maxCellSize, cellSize * (event.deltaY < 0 ? 1.2 : 1 / 1.2)));
  const after = screenToWorld(event.clientX - rectangle.left, event.clientY - rectangle.top);
  cameraX += before.column - after.column;
  cameraY += before.row - after.row;
  render();
}, { passive: false });

startButton.addEventListener("click", () => {
  if (running || liveCells.size === 0) return;
  running = true;
  animationFrame = requestAnimationFrame(animate);
});

pauseButton.addEventListener("click", () => {
  running = false;
  cancelAnimationFrame(animationFrame);
});

resetButton.addEventListener("click", () => {
  running = false;
  cancelAnimationFrame(animationFrame);
  liveCells.clear();
  generation = 0;
  updateGeneration();
  render();
});

window.addEventListener("resize", resizeCanvas);
updateGeneration();
resizeCanvas();
