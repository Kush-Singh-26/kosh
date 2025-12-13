---
title: "Math Animations"
date: "2025-12-12"
description: "A trial page to run math animation using opengl & wasm"
tags: ["Animation"]
pinned: false
---

This is just a trial.

## 1. Vector Addition Animation

- vectors A and B combine to form the resultant vector C

<wasm-sim 
    src="vector_sim" 
    controls='[
        {"id": "speed", "label": "Animation Speed", "min": 0, "max": 3, "val": 1, "step": 0.1}
    ]'>
</wasm-sim>

## 2. Dot Product & Projection

- The yellow vector shows the projection of A onto B (Green)

<wasm-sim 
    src="dot_product" 
    controls='[
        {"id": "ax", "label": "Vector A (x)", "min": -4, "max": 4, "val": 2, "step": 0.1},
        {"id": "ay", "label": "Vector A (y)", "min": -4, "max": 4, "val": 3, "step": 0.1}
    ]'>
</wasm-sim>

## 3. Linear Transformation

- Control the basis vectors i (Green) and j (Red) to see how they transform the grid and the vector v (Yellow).

<wasm-sim 
    src="linear_transform" 
    controls='[
        {"id": "ix", "label": "i-hat x", "min": -2, "max": 2, "val": 1, "step": 0.1},
        {"id": "iy", "label": "i-hat y", "min": -2, "max": 2, "val": 0, "step": 0.1},
        {"id": "jx", "label": "j-hat x", "min": -2, "max": 2, "val": 0, "step": 0.1},
        {"id": "jy", "label": "j-hat y", "min": -2, "max": 2, "val": 1, "step": 0.1},
        {"id": "vx", "label": "Vector v (x component)", "min": -3, "max": 3, "val": 2, "step": 0.1},
        {"id": "vy", "label": "Vector v (y component)", "min": -3, "max": 3, "val": 1, "step": 0.1}
    ]'>
</wasm-sim>

## 4. Vector Subtraction

- Visualizing A - B. The Red vector connects B to A.

<wasm-sim 
    src="vector_sub" 
    controls='[
        {"id": "ax", "label": "Vector A (x)", "min": -4, "max": 4, "val": 3, "step": 0.1},
        {"id": "ay", "label": "Vector A (y)", "min": -4, "max": 4, "val": 2, "step": 0.1},
        {"id": "bx", "label": "Vector B (x)", "min": -4, "max": 4, "val": 1, "step": 0.1},
        {"id": "by", "label": "Vector B (y)", "min": -4, "max": 4, "val": -1, "step": 0.1}
    ]'>
</wasm-sim>
<script src="static/js/wasm_engine.js"></script>