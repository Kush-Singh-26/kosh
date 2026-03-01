---
title: "1. Maths4ML: Vectors, Basis & Spans"
date: "2025-12-15"
description: "The Building Blocks – Vectors, Basis & Spans"
tags: ["Maths for ML"]
pinned: false
---

>This is a short series on basics of maths required for ML. The purpose of this series is to just get a quick refresher of maths and practice my latex skills and to implement more animations using my animation engine.

## The Space where data lives

The dataset is cloud of *points in space* and *vectors are the instructions to reach those points*.

To an ML algo, a row in a dataset is a specific point in a vast universe.
- If there are 2 columns, then the point will live in a 2D plane.
- It there are 100 columns, then the point will be found in a 100-dimensional hyperspace.

It is better to think **vectors as movements**. A vector $v = [3, 2]$, is an instruction to move 3 units East and 2 units North **from origin $(0, 0)$**.
- The arrow from $(0,0)$ to $(3,2)$ is the vector showing the movement.

> Thus, the point $(3,2)$ represents a vector from origin $(0,0)$.

### Moving data through space to transform it.

$$ \hat h = [size, bedrooms, age] $$

- This is a representation of a house in vector form.
- This vector is a position in a high-dimensional space.

Let,

$$ \hat h = [1500 \, sqft, 3 \, beds, 20 \, yrs] $$

- This house vector represents where the house currently sits in feature space.
    - This is the starting point.

- Some renovation is done on the house. It will change the house which can be represented as :

$$ \hat r = [+300 \, sqft, +1 \, bed, -5 \, yrs] $$

So, when these 2 vectors are added :

$$ \hat h_{new} = \hat h + \hat r = [1800, 4, 15] $$

> The house has been **moved** in feature space.

If the price of the house was $\text{price} = \hat w \cdot \hat h$, ($\hat w$ is importance of each vector), then renovation will change the price

> So renovations, upgrades, trends, or learned transformations all become vector movements.

Thus, model operations are movements and the aim is to find the right direction to move the data.

## Basis Vectors

Now that it is established vectors are movements, how to define or describe that movement.

The directions *East* or `x-axis` and *North* or `y-axis` are formalized into **Basis vectors**.

1. $\hat i$ : unit vector (length = 1) pointing purely to right.
2. $\hat j$ : unit vector pointing purely up.

So, $v = [3, 2]$ means take 3 of $\hat i$ and chain them together and then take 2 of $\hat j$ and add them.

> Geometrically, **any vector in 2D space is just a "linear combination" (a mix) of these two fundamental arrows**.

## Span

If instead of *East* and *North* directions 2 random vectors $v$ and $w$ are given :
- $v$ points North-East
- $w$ points South-East

Given that we can stretch, flip, shrink and add these 2 vectors as much as possible, the **collection of all reachable points is called the Span**.

### Linearly Independent

If $v$ and $w$ point in different directions, it is possible to combine them to reach any point on the 2D plane.

> Thus, the span is the entire 2D plane = $\mathbb{R}^2$.

If grid lines are drawn on the basis of these 2 vectors, then a skewed mesh will be observed which will cover everything.

### Linearly Independent

If both $v$ and $w$ point in same direction (like East) but with different magnitude, then it is only possible to move West or East.

- So even though there are 2 vectors, we are stuck on a single 1D line.
- The span has collapsed.

This happened because the vectors are linearly dependent. This means there is redundancy.

---

So, the formal definitions of all the concepts discussed above :

1. <u>**Column Vector**</u>

The movement instructions are written as column vectors.

$$ \vec{v} = \begin{bmatrix} x \\ y \end{bmatrix} $$

- A more general definition of vectors is :
    - ordered finite lists of numbers.
    - a type of mathematical object that can be added together and/or multiplied by a number to obtain another object of the same kind.

2. <u>**Linear Combination**</u>

Any vector in a space can be defined as a *scaling* of the basis vectors $\hat i$ and $\hat j$.

$$ \vec{v} = x\hat i + y \hat j = x \begin{bmatrix} 1 \\ 0 \end{bmatrix} + y \begin{bmatrix} 0 \\ 1 \end{bmatrix}$$

3. <u>**Span**</u>

The span of a set of vectors $ \{v_1, v_2, \cdots, v_k \} $ is the set of all vectors $y$ that can be created by :

$$ y = c_1 v_1 + c_2 v_2 + \cdots + c_k v_k  $$

- $c$ is a real number.
- If the span of 2 vectors is a plane, they are **linearly independent**.
- If the span of 2 vectors is a line, they are **linearly dependent**.

4. <u>**Vector space**</u>

It is a set of proper vectors and all possible linear combinatios of the vector set.

5. <u>**Vector Subspace**</u>

A vector subspace (or linear subspace) is a vector space that lies within a larger vector space.

- Contains the zero vector
- Closure under addition and multiplication


## Span Explorer

- The red vector ($v$) denotes *right* in this universe.
- The blue vector ($w$) denoted *up*.

> Drag the red and blue arrows and make the green arrow touch the target.

<wasm-sim src="span_explorer">
  <script type="application/json">
    [
        {"id": "c_red", "label": "Red Scale", "min": -3, "max": 3, "step": 0.1, "val": 1.0},
        {"id": "c_blue", "label": "Blue Scale", "min": -3, "max": 3, "step": 0.1, "val": 1.0}
    ]
  </script>
</wasm-sim>

Under the standard basis vectors ($\hat i$ and $\hat j$), the graph will have perfect squares as these basis vectors are orthogonal and unit legth.

Dragging the arrows will change the definition of the space.
- **Dragging the vectors' length** : Stretch Red Arrow to be 2 units long signifies that one step in the red direction in new world covers 2 units of distance.
    - The grid lines spread apart to reflect this.

- **Dragging Angle** : Tilting the blue arrow to $45 \degree$ angle to red arrow tells that moving up in the new world also moves a bit right.

> - The background mesh or the grid is made by repeating these arrows over and over.
>- The intersection of these lines creates the grid points. Every intersection represents a reachable destination using whole numbers (e.g., "3 Red steps + 2 Blue steps").

### Linear Dependence

If the Blue Arrow ($\vec{w}$) so it sits exactly on top of the Red Arrow ($\vec{v}$, instead of a net / mesh a stringht line will be observed.
- The area of the diamonds will become 0.

Others wise, the the vectors *span* the entire plane as it possible to reach every point in the space by just adjusting the basis vectors.

---

$ \therefore $ Provided 2 vectors $\vec{v}$ & $\vec{w}$ are linearly independent, they will **span** the entire 2D plane ($\mathbb{R}^2$), i.e., it is possible to reach any point in space by sliding the sliders above.

<script src="static/js/wasm_engine.js"></script>