const boardElement = document.querySelector("#board");
const generationElement = document.querySelector("#generation");
const statusElement = document.querySelector("#status");
let generation = 0;

async function loadBoard(endpoint, options = {}) {
  try {
    const response = await fetch(endpoint, options);
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    const board = await response.json();
    renderBoard(board);
    statusElement.textContent = "";
  } catch (error) {
    statusElement.textContent = `Erreur : ${error.message}`;
  }
}

function renderBoard(board) {
  boardElement.replaceChildren();
  for (let row = 0; row < board.height; row += 1) {
    const tableRow = document.createElement("tr");
    for (let column = 0; column < board.width; column += 1) {
      const cell = document.createElement("td");
      cell.textContent = board.cells[row * board.width + column] ? "X" : ".";
      tableRow.appendChild(cell);
    }
    boardElement.appendChild(tableRow);
  }
}

document.querySelector("#step").addEventListener("click", async () => {
  await loadBoard("/api/tick", { method: "POST" });
  generation += 1;
  generationElement.textContent = generation;
});

document.querySelector("#reset").addEventListener("click", async () => {
  await loadBoard("/api/reset", { method: "POST" });
  generation = 0;
  generationElement.textContent = generation;
});

loadBoard("/api/state");
